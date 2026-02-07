package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
	RTCInt = machine.D12
)

type Hypnos struct {
	rtc     *ds3231.Device
	name    string
	version string
}

func (h *Hypnos) Name() string { return h.name + " " + h.version }

func (h *Hypnos) ReadFile(name string) ([]byte, error) {
	// TODO: read file from SD card
	return nil, errors.New("hypnos: sd card not yet implemented")
}

func (h *Hypnos) ReadTime() (time.Time, error) {
	return h.rtc.ReadTime()
}

func (h *Hypnos) Sleep(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	reason, err := h.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		reason = sleepFallback(sampleInterval)
	}

	if reason&hardware.WakeSample != 0 {
		// Need to power on rails to give sensors power.
		// This isn't necessary for WakeHeartbeat reason
		// because we will just send a keep alive message
		// using network card.
		powerOn()
	}

	return reason, err
}

func ISR(p machine.Pin) {
	// No reason to check this error becuase there is nothing we could do with it.
	_ = RTCInt.SetInterrupt(0, nil)

	hardware.DisableWakeOnInterrupt(RTCInt)
}

func (h *Hypnos) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	// TODO: explaination from datasheet
	RTCInt.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Register interrupt.
	err := RTCInt.SetInterrupt(machine.PinFalling, ISR)
	if err != nil {
		return 0, err
	}

	// Switch GCLK_EIC to a low-power oscillator that keeps running in
	// standby so the EIC can detect the falling edge from the DS3231.
	hardware.ConfigureEICStandby()

	hardware.EnableWakeOnInterrupt(RTCInt)

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
	hardware.EnterStandby() // System hangs here until interrupt.

	// TODO: check which alarm triggered wakeup and change reason
	reason := hardware.WakeSample

	return reason, nil
}

func sleepFallback(sample time.Duration) hardware.WakeReason {
	powerOff()
	time.Sleep(sample)
	return hardware.WakeSample
}

const (
	// Number of times to retry I2C operations during probe.
	probeRetries = 3

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C) (*Hypnos, error) {
	configureRails()
	powerOn()

	rtc, err := configureRTC(bus)
	if err != nil {
		return nil, err
	}

	// TODO: detect board version during probe.
	return &Hypnos{rtc: rtc, name: "Hypnos 3.3"}, nil
}

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn() {
	Rail3V.Low() // Hypnos 3.3V rail is active-low
	Rail5V.High()
	machine.LED.High()
	time.Sleep(time.Second) // Give time for rails to turn on
}

func powerOff() {
	Rail3V.High()
	Rail5V.Low()
	machine.LED.Low()
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

	return &rtc, err
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
