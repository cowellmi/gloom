package hypnos

import (
	"errors"
	"machine"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
)

type Hypnos struct {
	RTC ds3231.Device
}

// Creates a new hypnos connection. The I2C bus must already be configured.
func New(bus drivers.I2C) (*Hypnos, error) {
	var err error

	defer func() {
		if err != nil {
			railsOff()
		}
	}()

	// Initialize rails.
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	railsOn()

	rtc := ds3231.New(machine.I2C0)

	if !rtc.Configure() {
		err = errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	// If the Hypnos isn't connected or the DS3231 is malfunctioning,
	// this is where error might occur.
	err = rtc.SetRunning(true)
	if err != nil {
		return nil, err
	}

	err = rtc.SetSqwPinMode(ds3231.SqwPinMode(ds3231.ModeAlarmBoth))
	if err != nil {
		return nil, err
	}

	return &Hypnos{RTC: rtc}, nil
}

// Hypnos implements gloom.Sleeper interface.
func (h *Hypnos) Sleep() error {
	railsOff()

	return nil
}

func railsOn() {
	Rail3V.Low()  // Hypnos 3.3V is Active-Low
	Rail5V.High() // Hypnos 5V is Active-High
	time.Sleep(50 * time.Millisecond)
}

func railsOff() {
	Rail3V.High()
	Rail5V.Low()
	time.Sleep(50 * time.Millisecond)
}
