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
	powerOff()

	reason, err := h.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		reason = sleepFallback(sampleInterval)
	}

	// Only restore sensor power rails for a sample wake.
	if reason&hardware.WakeSample != 0 {
		powerOn()
		if waitErr := waitForRTC(h.rtc); waitErr != nil {
			err = waitErr
		}
	}

	return reason, err
}

func (h *Hypnos) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	time.Sleep(sampleInterval)

	// TODO: enter STANDBY mode

	return hardware.WakeSample, nil
}

func sleepFallback(sample time.Duration) hardware.WakeReason {
	if sample > 0 {
		time.Sleep(sample)
	}
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
	// Initialize power rails.
	configureRails()
	powerOn()

	rtc, err := configureRTC(bus)
	if err != nil {
		return nil, err
	}

	// TODO: detect board version during probe.
	return &Hypnos{rtc: rtc, version: "3.3"}, nil
}

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn() {
	Rail3V.Low()       // Hypnos 3.3V is Active-Low
	Rail5V.High()      // Hypnos 5V is Active-High
	machine.LED.High() // Visual indicator
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

	err := waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	err = rtc.SetSqwPinMode(ds3231.SQW_OFF)
	if err != nil {
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
