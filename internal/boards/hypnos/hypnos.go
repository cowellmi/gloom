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
	Name = "Hypnos"

	// Machine pin connected to the DS3231 RTC alarm output.
	AlarmPin = machine.D12

	// Number of times to retry I2C operations during probe.
	probeRetries = 3

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

type Board struct {
	proc    mcu.MCU
	rtc     *ds3231.Device
	version string
}

// Probe I2C for Hypnos components. The I2C bus must already be
// configured. proc provides the processor-level sleep primitives.
func Probe(bus drivers.I2C, proc mcu.MCU) (*Board, error) {
	configureRails()
	powerOn()

	rtc, err := configureRTC(bus)
	if err != nil {
		return nil, err
	}

	// TODO: detect board version during probe.
	return &Board{proc: proc, rtc: rtc, version: "3.3"}, nil
}

func (b *Board) Identifier() string { return Name + " " + b.version }

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

	if reason&hal.WakeSample != 0 {
		// Need to power on rails to give sensors power.
		// This isn't necessary for WakeHeartbeat reason
		// because we will just send a keep alive message
		// using network card.
		powerOn()
		if rtcErr := waitForRTC(b.rtc); rtcErr != nil {
			return reason, errors.Join(err, rtcErr)
		}
	}

	return reason, err
}

func (b *Board) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hal.WakeReason, error) {
	AlarmPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Register interrupt. The first call also initializes the EIC
	// peripheral and its default GCLK0 clock routing in TinyGo.
	err := AlarmPin.SetInterrupt(machine.PinFalling, b.alarmISR)
	if err != nil {
		return 0, err
	}

	// Switch GCLK_EIC to OSCULP32K so edge detection works in standby.
	// Only needed once; the routing persists across sleep/wake cycles.
	b.proc.PrepareStandby()

	b.proc.EnableWake(AlarmPin)

	// Clear any pending alarms.
	if err := b.rtc.ClearAlarm1(); err != nil {
		return 0, err
	}
	if err := b.rtc.ClearAlarm2(); err != nil {
		return 0, err
	}

	// Read current time and set alarms based on intervals.
	//
	// TODO: maybe we should subtract a couple seconds to account
	// for the time.Sleep delay for turning the rails back on.
	// For example, currently a 5 second interval is more like
	// 7 seconds due to delays.
	now, err := b.rtc.ReadTime()
	if err != nil {
		return 0, err
	}

	target := now.Add(sampleInterval)
	if err := b.rtc.SetAlarm1(target, ds3231.A1_DATE); err != nil {
		return 0, err
	}

	// Kill power to the MOSFET rails.
	powerOff()

	// Enter deep sleep. Execution halts here until interrupt.
	b.proc.Standby()

	// TODO: check which alarm triggered wakeup and change reason
	reason := hal.WakeSample

	return reason, nil
}

func (b *Board) sleepIdle(sample time.Duration) hal.WakeReason {
	powerOff()
	time.Sleep(sample)
	return hal.WakeSample
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

// clearAlarmInterrupt tears down the RTC alarm interrupt registration.
// Safe to call whether or not an interrupt is currently registered.
func (h *Board) clearAlarmInterrupt() {
	_ = AlarmPin.SetInterrupt(0, nil)
	h.proc.DisableWake(AlarmPin)
}

func (h *Board) alarmISR(p machine.Pin) {
	h.clearAlarmInterrupt()
}
