package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hardware/platform"
	"github.com/cowellmi/gloom/internal/hardware/samd"
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
	rtc           *ds3231.Device
	version       string
	eicConfigured bool
	ledEnabled    bool
}

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C) (*Board, error) {
	configureRails()
	powerOn()

	rtc, err := configureRTC(bus)
	if err != nil {
		return nil, err
	}

	// TODO: detect board version during probe.
	return &Board{rtc: rtc, version: "3.3"}, nil
}

func (b *Board) EnableLED() {
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
	b.ledEnabled = true
}

func (h *Board) Name() string { return Name + " " + h.version }

func (h *Board) ReadFile(name string) ([]byte, error) {
	// TODO: read file from SD card
	return nil, errors.New("hypnos: sd card not yet implemented")
}

func (h *Board) ReadTime() (time.Time, error) {
	return h.rtc.ReadTime()
}

func (h *Board) Sleep(sampleInterval, heartbeatInterval time.Duration) (platform.WakeReason, error) {
	reason, err := h.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		clearAlarmInterrupt()
		reason = h.sleepIdle(sampleInterval)
		err = errors.New("hypnos: standby failed: " + err.Error())
	}

	if reason&platform.WakeSample != 0 {
		// Need to power on rails to give sensors power.
		// This isn't necessary for WakeHeartbeat reason
		// because we will just send a keep alive message
		// using network card.
		powerOn()
		if rtcErr := waitForRTC(h.rtc); rtcErr != nil {
			return reason, errors.Join(err, rtcErr)
		}
	}

	return reason, err
}

func (h *Board) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (platform.WakeReason, error) {
	AlarmPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Register interrupt. The first call also initializes the EIC
	// peripheral and its default GCLK0 clock routing in TinyGo.
	err := AlarmPin.SetInterrupt(machine.PinFalling, alarmISR)
	if err != nil {
		return 0, err
	}

	// Switch GCLK_EIC to OSCULP32K so edge detection works in standby.
	// Only needed once; the GCLK6 routing persists across sleep/wake
	// cycles and is not touched by EnterStandby or subsequent
	// SetInterrupt calls (TinyGo only inits EIC once).
	if !h.eicConfigured {
		samd.ConfigureEICStandby()
		h.eicConfigured = true
	}

	samd.EnableWakeOnInterrupt(AlarmPin)

	// Clear any pending alarms.
	if err := h.rtc.ClearAlarm1(); err != nil {
		return 0, err
	}
	if err := h.rtc.ClearAlarm2(); err != nil {
		return 0, err
	}

	// Read current time and set alarms based on intervals.
	//
	// TODO: maybe we should subtract a couple seconds to account
	// for the time.Sleep delay for turning the rails back on.
	// For example, currently a 5 second interval is more like
	// 7 seconds due to delays.
	now, err := h.rtc.ReadTime()
	if err != nil {
		return 0, err
	}

	target := now.Add(sampleInterval)
	if err := h.rtc.SetAlarm1(target, ds3231.A1_DATE); err != nil {
		return 0, err
	}

	// Kill power to the MOSFET rails.
	powerOff()

	// Enter SAMD21 standby sleep. This detaches USB, disables SysTick,
	// issues WFI, and restores USB + SysTick on wake.
	samd.EnterStandby() // System hangs here until interrupt.

	// TODO: check which alarm triggered wakeup and change reason
	reason := platform.WakeSample

	return reason, nil
}

func (h *Board) sleepIdle(sample time.Duration) platform.WakeReason {
	powerOff()
	time.Sleep(sample)
	return platform.WakeSample
}

func configureRTC(bus drivers.I2C) (*ds3231.Device, error) {
	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err := errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	// It way take a few trys to establish I2C connection after powering on.
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
