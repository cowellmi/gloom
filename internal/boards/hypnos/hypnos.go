package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/mcu"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	// Machine pin connected to the DS3231 RTC alarm output.
	AlarmPin = machine.D12

	// Number of attempts to retry I2C operations during probe.
	probeRetries = 5

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond

	powerOnDelay = 2 * time.Second
)

type Board struct {
	proc          mcu.MCU
	rtc           *ds3231.Device
	version       string
	nextSample    time.Time
	nextHeartbeat time.Time
}

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C, proc mcu.MCU) (*Board, error) {
	configureRails()
	powerOn33()
	powerOn5()
	time.Sleep(powerOnDelay)

	rtc, err := configureRTC(bus)
	if err != nil {
		return nil, err
	}

	// TODO: detect board version during probe.
	//
	// Maybe when probing for SD card reader?
	// VERSION	| CHIP SELECT PIN
	// 3.3 		| 11
	// 3.2 		| 10

	proc.EnableWatchdog()

	b := &Board{proc: proc, rtc: rtc, version: "3.3"}

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
			sleepErrs = append(sleepErrs, prepErr)
		}

		b.proc.PetWatchdog()

		// Turn off the power rails either way.
		powerOff33()
		powerOff5()

		// If nothing went wrong, enter standby mode.
		if prepErr != nil {
			time.Sleep(time.Until(target))
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
		//
		// NOTE: is it safe to run after prepareForStandby failed and
		// time.Sleep executed?
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
	}
	if heartbeatInterval > 0 && !b.nextHeartbeat.IsZero() && !now.Before(b.nextHeartbeat) {
		reason = hal.WakeHeartbeat
		b.nextHeartbeat = time.Time{}
	} else {
		// If neither deadline was met, there must have been an external interrupt.
		reason = hal.WakeExternal
	}

	if reason != hal.WakeHeartbeat {
		// Turn on 5V power rails to power on sensors.
		powerOn5()
		time.Sleep(powerOnDelay)

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

func waitForRTC(rtc *ds3231.Device) error {
	for attempt := 0; attempt < probeRetries; attempt++ {
		err := rtc.SetRunning(true)
		if err == nil {
			return nil
		}
		time.Sleep(probeRetryDelay)
	}

	return errors.New("hypnos: rtc communication timed out")
}

func configureRTC(bus drivers.I2C) (*ds3231.Device, error) {
	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err := errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	// It may take a few tries to establish I2C connection after powering on.
	err := waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	// Clear any pending alarms.
	if err := rtc.ClearAlarm1(); err != nil {
		return nil, err
	}
	if err := rtc.ClearAlarm2(); err != nil {
		return nil, err
	}

	if err := rtc.SetSqwPinMode(ds3231.SQW_OFF); err != nil {
		return nil, err
	}

	return &rtc, nil
}
