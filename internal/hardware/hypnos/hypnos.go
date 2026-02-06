package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

type Board struct {
	rtc *ds3231.Device
}

func (h *Board) Name() string { return "Hypnos" }

func (h *Board) ReadFile(name string) ([]byte, error) {
	// TODO: read file from SD card
	return nil, errors.New("hypnos: sd card not yet implemented")
}

func (h *Board) ReadTime() (time.Time, error) {
	return h.rtc.ReadTime()
}

func (h *Board) Sleep(sample, heartbeat time.Duration) (hardware.WakeReason, error) {
	railsOff()

	time.Sleep(sample) // TODO: use RTC alarms + deep sleep
	reason := hardware.WakeSample

	// Only restore sensor power rails for a sample wake.
	if reason == hardware.WakeSample {
		railsOn()
	}

	if err := waitForRTC(h.rtc); err != nil {
		return reason, err
	}

	return reason, nil
}

const (
	// Number of times to retry I2C operations during probe.
	probeRetries = 3

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C) (*Board, error) {
	var err error

	defer func() {
		if err != nil {
			railsOff() // Reset machine pins.
		}
	}()

	// Initialize power rails.
	configureRails()
	railsOn()

	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err = errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	err = waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	err = rtc.SetSqwPinMode(ds3231.SqwPinMode(ds3231.ModeAlarmBoth))
	if err != nil {
		return nil, err
	}

	return &Board{rtc: &rtc}, nil
}

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
)

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
