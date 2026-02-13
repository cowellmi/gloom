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

	powerOnRails5Delay = 2 * time.Second
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
	time.Sleep(powerOnRails5Delay)

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
	reason, err := b.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		b.clearAlarmInterrupt()
		reason = b.sleepIdle(sampleInterval)
		err = errors.New("hypnos: standby failed: " + err.Error())
	}

	if reason == hal.WakeSample {
		// Need to power on 5v rails to give sensors power. This isn't
		// necessary for WakeHeartbeat reason because we will just send a
		// keep alive message using network card (no need to turn on sensors).
		powerOn5()
		time.Sleep(powerOnRails5Delay)
		if rtcErr := waitForRTC(b.rtc); rtcErr != nil {
			return reason, errors.Join(err, rtcErr)
		}
	}

	return reason, err
}

func (b *Board) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hal.WakeReason, error) {
	now, err := b.rtc.ReadTime()
	if err != nil {
		return 0, err
	}

	// Reset any deadline that hasn't been set yet (either first call
	// or was zeroed after being met on the previous cycle).
	if sampleInterval > 0 && b.nextSample.IsZero() {
		// Sample interval must account for delay to power on 5V rails.
		b.nextSample = now.Add(sampleInterval - powerOnRails5Delay)
	}
	if heartbeatInterval > 0 && b.nextHeartbeat.IsZero() {
		b.nextHeartbeat = now.Add(heartbeatInterval)
	}

	// Pick the earliest deadline.
	target := earliest(b.nextSample, b.nextHeartbeat)

	// Sleep unless a deadline has already passed while the MCU was awake.
	if target.IsZero() || now.Before(target) {
		AlarmPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

		// Register interrupt. The first call also initializes the EIC
		// peripheral and its default GCLK0 clock routing in TinyGo.
		err = AlarmPin.SetInterrupt(machine.PinFalling, b.alarmISR)
		if err != nil {
			return 0, err
		}

		// Setup MCU.
		b.proc.PrepareStandby()
		b.proc.EnableWake(AlarmPin)

		// Set the single RTC alarm if a timed interval is active.
		if !target.IsZero() {
			if err := b.rtc.ClearAlarm1(); err != nil {
				return 0, err
			}
			if err := b.rtc.SetAlarm1(target, ds3231.A1_DATE); err != nil {
				return 0, err
			}
		}

		// Kill power to the MOSFET rails.
		powerOff33()
		powerOff5()

		// Enter deep sleep. Execution halts here until interrupt.
		b.proc.Standby()

		// Turn on 3.3V rails which power the RTC.
		powerOn33()
		if err := waitForRTC(b.rtc); err != nil {
			return 0, err
		}

		// Clear the alarm flag so the INT pin releases back to HIGH.
		// Best-effort: if this fails the pre-sleep ClearAlarm1 on the
		// next cycle will retry before we re-enter standby.
		_ = b.rtc.ClearAlarm1()

		// Re-read RTC time after wake.
		now, err = b.rtc.ReadTime()
		if err != nil {
			return 0, err
		}
	}

	// Return the first met deadline. If both are met simultaneously the
	// second will fire on the next cycle (immediate, no sleep).
	if sampleInterval > 0 && !b.nextSample.IsZero() && !now.Before(b.nextSample) {
		b.nextSample = time.Time{}
		return hal.WakeSample, nil
	}
	if heartbeatInterval > 0 && !b.nextHeartbeat.IsZero() && !now.Before(b.nextHeartbeat) {
		b.nextHeartbeat = time.Time{}
		return hal.WakeHeartbeat, nil
	}

	return hal.WakeExternal, nil
}

func (b *Board) sleepIdle(sample time.Duration) hal.WakeReason {
	time.Sleep(sample)
	return hal.WakeSample
}

// earliest returns the earlier of two times. If either is zero (unscheduled),
// the other is returned. If both are zero, zero is returned.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() && b.IsZero() {
		return time.Time{}
	}
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
