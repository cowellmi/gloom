package hypnos

import (
	"device/arm"
	"device/sam"
	"errors"
	"machine"
	"runtime/volatile"
	"time"
	"unsafe"

	"github.com/cowellmi/gloom/internal/hardware"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
	RTCInt = machine.D12 // DS3231 INT/SQW -> PA19 / EXTINT3
)

type Hypnos struct {
	rtc     *ds3231.Device
	version string
}

func (h *Hypnos) Name() string { return "Hypnos " + h.version }

func (h *Hypnos) ReadFile(name string) ([]byte, error) {
	// TODO: read file from SD card
	return nil, errors.New("hypnos: sd card not yet implemented")
}

func (h *Hypnos) ReadTime() (time.Time, error) {
	return h.rtc.ReadTime()
}

func (h *Hypnos) Sleep(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	railsOff()

	reason, err := h.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		// RTC alarm setup failed. Fall back to time.Sleep so
		// the device keeps collecting data without entering
		// standby mode.
		reason = sleepFallback(sampleInterval)
	}

	// Only restore sensor power rails for a sample wake.
	if reason&hardware.WakeSample != 0 {
		railsOn()
		if waitErr := waitForRTC(h.rtc); waitErr != nil {
			err = waitErr
		}
	}

	return reason, err
}

// sleepStandby arms the DS3231 alarms and enters SAMD21
// standby mode. Returns an error if any RTC operation fails.
func (h *Hypnos) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	// Read current time for computing alarm targets.
	now, err := h.rtc.ReadTime()
	if err != nil {
		return 0, err
	}

	// Clear any pending alarm flags so the INT pin is released
	// (pulled HIGH) before we arm new alarms and enter standby.
	h.rtc.ClearAlarm1()
	h.rtc.ClearAlarm2()

	// Arm Alarm 1 (sample interval, second resolution).
	if sampleInterval > 0 {
		target := now.Add(sampleInterval)
		err := h.rtc.SetAlarm1(target, alarm1Mode(sampleInterval))
		if err != nil {
			return 0, err
		}
	} else {
		err := h.rtc.SetEnabledAlarm1(false)
		if err != nil {
			return 0, err
		}
	}

	// Arm Alarm 2 (heartbeat interval, minute resolution).
	if heartbeatInterval > 0 {
		target := now.Add(heartbeatInterval)
		// Alarm 2 has no seconds field. Round up to the next
		// whole minute so the alarm never fires early.
		if target.Second() > 0 {
			target = target.Add(time.Duration(60-target.Second()) * time.Second)
		}
		err := h.rtc.SetAlarm2(target, alarm2Mode(heartbeatInterval))
		if err != nil {
			return 0, err
		}
	} else {
		err := h.rtc.SetEnabledAlarm2(false)
		if err != nil {
			return 0, err
		}
	}

	// Arm the EIC interrupt for the RTC INT pin (falling edge).
	RTCInt.SetInterrupt(machine.PinFalling, func(machine.Pin) {})

	// Enter SAMD21 standby (deep sleep). Execution halts here
	// until the DS3231 INT pin pulls low.
	standby()

	// Disarm the EIC interrupt.
	RTCInt.SetInterrupt(0, nil)

	// Determine which alarm(s) woke us by reading the DS3231
	// status register. Both can fire between cycles, so we
	// check each independently.
	//
	// If neither flag is set the wake was either a sensor
	// interrupt or the status read failed over I2C.
	reason := hardware.WakeSample
	if h.rtc.IsAlarm1Fired() || h.rtc.IsAlarm2Fired() {
		reason = 0
		if h.rtc.IsAlarm1Fired() {
			reason |= hardware.WakeSample
		}
		if h.rtc.IsAlarm2Fired() {
			reason |= hardware.WakeHeartbeat
		}
	}

	// Clear alarm flags so the INT pin is released.
	h.rtc.ClearAlarm1()
	h.rtc.ClearAlarm2()

	return reason, nil
}

// sleepFallback uses time.Sleep when the RTC is unavailable.
// Less power-efficient than standby, but keeps the device
// running. Both sample and heartbeat fire every cycle since
// we can't time them independently without the RTC.
func sleepFallback(sample time.Duration) hardware.WakeReason {
	if sample > 0 {
		time.Sleep(sample)
	}
	return hardware.WakeSample | hardware.WakeHeartbeat
}

const (
	// Number of times to retry I2C operations during probe.
	probeRetries = 3

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C) (*Hypnos, error) {
	var err error

	defer func() {
		if err != nil {
			railsOff() // Reset machine pins.
		}
	}()

	// Initialize power rails.
	configureRails()
	railsOn()

	// Configure the RTC interrupt pin as an input with pull-up.
	// The DS3231 INT/SQW output is open-drain and active-low.
	RTCInt.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Enable EIC async wake-up for the RTC interrupt pin so the
	// MCU can exit standby when the DS3231 fires an alarm.
	// D12 = PA19 -> EXTINT3.
	sam.EIC.WAKEUP.SetBits(1 << 3)

	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err = errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	err = waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	// Switch the DS3231 SQW/INT pin to alarm interrupt mode.
	err = rtc.SetSqwPinMode(ds3231.SqwPinMode(ds3231.ModeAlarmBoth))
	if err != nil {
		return nil, err
	}

	// TODO: detect board revision during probe.
	return &Hypnos{rtc: &rtc, version: "3.3"}, nil
}

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func railsOn() {
	Rail3V.Low()  // Hypnos 3.3V is Active-Low
	Rail5V.High() // Hypnos 5V is Active-High
}

func railsOff() {
	Rail3V.High()
	Rail5V.Low()
}

// I2C can produce transient bus errors right after power is
// restored while pull-ups and the oscillator stabilize. Ping the
// RTC until it responds, same as during initial probe.
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

// alarm1Mode returns the appropriate Alarm 1 match mode for
// the given interval. Intervals >= 24h include date matching
// to avoid firing 24h early on a time-only match.
func alarm1Mode(d time.Duration) ds3231.Alarm1Mode {
	if d >= 24*time.Hour {
		return ds3231.A1_DATE
	}
	return ds3231.A1_HOUR
}

// alarm2Mode returns the appropriate Alarm 2 match mode for
// the given interval (minute resolution only).
func alarm2Mode(d time.Duration) ds3231.Alarm2Mode {
	if d >= 24*time.Hour {
		return ds3231.A2_DATE
	}
	return ds3231.A2_HOUR
}

// ── SAMD21 Standby ─────────────────────────────────────────

// ARM System Control Block - System Control Register.
// Cortex-M0+ SCB base = 0xE000ED00, SCR offset = 0x10.
var scr = (*volatile.Register32)(unsafe.Pointer(uintptr(0xE000ED10)))

const scrSleepDeep = 1 << 2

// standby puts the SAMD21 into standby (deep sleep) mode.
// Execution resumes after WFI when an enabled interrupt fires.
func standby() {
	scr.SetBits(scrSleepDeep)
	arm.Asm("dsb 0xf") // Ensure all memory accesses complete.
	arm.Asm("wfi")     // Wait For Interrupt - CPU halts here.
	scr.ClearBits(scrSleepDeep)
}
