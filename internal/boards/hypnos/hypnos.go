package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/mcu"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/wait"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	// Machine pin connected to the DS3231 RTC alarm output.
	AlarmPin = machine.D12

	// Delay required after powering on 5V rails.
	powerOnDelay = 2 * time.Second

	// Delay to hold rails off during the initial power cycle.
	// Must be long enough for the SD card's internal capacitors to
	// fully discharge so its SPI state machine resets. 250 ms is
	// conservative; most cards discharge in under 100 ms.
	sdPowerCycleDelay = 250 * time.Millisecond
)

type Board struct {
	proc          mcu.MCU
	rtc           *ds3231.Device
	Card          *sdcard.Card
	version       string
	nextSample    time.Time
	nextHeartbeat time.Time
}

// Probe detects and initialises all Hypnos components: RTC (via I2C)
// and SD card reader (via SPI). The I2C bus must already be configured.
// Returns a fatal error if either component is missing.
//
// The caller should enable the hardware watchdog before calling Probe
// so that SPI/I2C hangs (e.g. corrupted SD card) cause a reset
// instead of a permanent hang.
func Probe(bus drivers.I2C, spi *machine.SPI, sck, sdo, sdi machine.Pin, proc mcu.MCU) (*Board, error) {
	configureRails()

	// Force a clean power cycle so the SD card resets its SPI state
	// machine. After a WDT reset or unexpected power loss mid-write,
	// the card can be stuck waiting for the rest of a previous SPI
	// command. The Mac won't see this (it uses SDIO with a hardware
	// reset line) but SPI mode has no out-of-band reset -- the only
	// fix is to cut power long enough for the card's internal caps
	// to discharge.
	powerOff33()
	powerOff5()
	wait.For(sdPowerCycleDelay)

	powerOn33()
	powerOn5()
	wait.For(powerOnDelay)

	proc.PetWatchdog()

	rtc, err := probeRTC(bus)
	if err != nil {
		return nil, err
	}

	proc.PetWatchdog()

	// Detect board version by probing the SD card reader on each
	// known chip-select pin (D11 for v3.3, D10 for v3.2).
	card, version, sdErr := probeSDCard(spi, sck, sdo, sdi)
	if sdErr != nil {
		return nil, sdErr
	}

	proc.PetWatchdog()

	b := &Board{proc: proc, rtc: rtc, Card: card, version: version}
	return b, nil
}

func (b *Board) Identifier() string {
	return "Hypnos " + b.version + " (" + b.proc.Identifier() + ")"
}

func (b *Board) ReadTime() (time.Time, error) {
	return b.rtc.ReadTime()
}

func (b *Board) Sleep(sampleInterval, heartbeatInterval time.Duration) (hal.WakeReason, error) {
	// Keep track of sleep errors with graceful degradation.
	var sleepErrs []error

	b.proc.PetWatchdog()

	// Get the current time to calculate next intervals.
	now, err := b.rtc.ReadTime()
	if err != nil {
		sleepErrs = append(sleepErrs, err)
		// Fallback to internal clock.
		now = time.Now()
	}

	b.proc.PetWatchdog()

	// Reset any deadline that hasn't been set yet (either first call
	// or was zeroed after being met on the previous cycle).
	if sampleInterval > 0 && b.nextSample.IsZero() {
		// Sample interval must account for delay to restore power to the
		// 5V rails.
		b.nextSample = now.Add(sampleInterval - powerOnDelay)
	}
	if heartbeatInterval > 0 && b.nextHeartbeat.IsZero() {
		b.nextHeartbeat = now.Add(heartbeatInterval)
	}

	// Pick the earliest deadline.
	target := earliest(b.nextSample, b.nextHeartbeat)

	// Go to sleep unless time now has already elapsed target. A target with
	// a zero value indicates both nextSample and nextHeartbeat are not set.
	// If neither are set the device still goes into deepsleep and waits for
	// and external interrupt (e.g. a sensor like tipping bucket).
	//
	// NOTE:
	// We might want to check if an external sensor interrupt has been registered.
	// We could add a []sensorInterrupts to the Sleep function arguments.
	if now.Before(target) || target.IsZero() {
		b.proc.PetWatchdog()

		// Check if anything goes wrong while preparing for standby mode.
		prepErr := b.prepareForStandby(target)
		if prepErr != nil {
			b.clearAlarmInterrupt()
			sleepErrs = append(sleepErrs, prepErr)
		}

		b.proc.PetWatchdog()

		// Turn off the power rails either way.
		powerOff33()
		powerOff5()

		// If nothing went wrong, enter standby mode. Otherwise fall
		// back to an idle sleep loop that periodically pets the watchdog.
		if prepErr != nil {
			b.idleSleep(target)
		} else {
			b.proc.Standby()
		}

		b.proc.PetWatchdog()

		// Turn on 3.3V rails.
		powerOn33()
		if err := waitForRTC(b.rtc); err != nil {
			sleepErrs = append(sleepErrs, err)
		}

		b.proc.PetWatchdog()

		// Clear the alarm flag so the INT pin releases back to HIGH.
		// Best-effort: if this fails the pre-sleep ClearAlarm1 on the
		// next cycle will retry before we re-enter standby.
		_ = b.rtc.ClearAlarm1()

		b.proc.PetWatchdog()

		// Re-read RTC time after wake.
		now, err = b.rtc.ReadTime()
		if err != nil {
			sleepErrs = append(sleepErrs, err)
			now = time.Now()
		}

		b.proc.PetWatchdog()
	}

	var reason hal.WakeReason

	// Return the first met deadline. If both are met simultaneously the
	// second will fire on the next cycle before going back to sleep because
	// of the now.Before(target) check when going to sleep.
	if sampleInterval > 0 && !b.nextSample.IsZero() && !now.Before(b.nextSample) {
		reason = hal.WakeSample
		b.nextSample = time.Time{}
	} else if heartbeatInterval > 0 && !b.nextHeartbeat.IsZero() && !now.Before(b.nextHeartbeat) {
		reason = hal.WakeHeartbeat
		b.nextHeartbeat = time.Time{}
	} else {
		// If neither deadline was met, there must have been an external interrupt.
		reason = hal.WakeExternal
	}

	if reason != hal.WakeHeartbeat {
		// Turn on 5V power rails to power on sensors. This isn't
		// necessary for WakeHeartbeat because we just send a keep alive
		// message using the network card (no need to turn on sensors).
		powerOn5()

		wait.For(powerOnDelay)

		b.proc.PetWatchdog()
	}

	return reason, errors.Join(sleepErrs...)
}

func (b *Board) prepareForStandby(target time.Time) error {
	AlarmPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Register interrupt. The first call also initializes the EIC
	// peripheral and its default GCLK0 clock routing in TinyGo.
	err := AlarmPin.SetInterrupt(machine.PinFalling, b.alarmISR)
	if err != nil {
		return err
	}

	// Setup MCU.
	b.proc.PrepareStandby()
	b.proc.EnableWake(AlarmPin)

	// Set the single RTC alarm if a timed interval is active.
	if !target.IsZero() {
		if err := b.rtc.ClearAlarm1(); err != nil {
			return err
		}
		if err := b.rtc.SetAlarm1(target, ds3231.A1_DATE); err != nil {
			return err
		}
	}

	return nil
}

// idleSleep is the fallback when standby preparation fails. It sleeps in
// short intervals, petting the watchdog between each, until the target time
// is reached. If target is zero (no timed deadline), it sleeps for a single
// watchdog-safe interval to avoid spinning.
func (b *Board) idleSleep(target time.Time) {
	const tick = 4 * time.Second // well within the ~8s watchdog timeout

	if target.IsZero() {
		wait.For(tick)
		return
	}

	for time.Now().Before(target) {
		b.proc.PetWatchdog()
		remaining := time.Until(target)
		if remaining > tick {
			remaining = tick
		}
		wait.For(remaining)
	}
}

// earliest returns the earlier of two times. If either is zero (unscheduled),
// the other is returned. If both are zero, zero is returned.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.Before(b) {
		return a
	}
	return b
}

// clearAlarmInterrupt tears down the RTC alarm interrupt registration.
// Safe to call whether or not an interrupt is currently registered.
func (b *Board) clearAlarmInterrupt() {
	_ = AlarmPin.SetInterrupt(0, nil)
	b.proc.DisableWake(AlarmPin)
}

func (b *Board) alarmISR(p machine.Pin) {
	b.clearAlarmInterrupt()
}

