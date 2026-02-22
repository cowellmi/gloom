package main

import (
	"io"
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// Board holds board-specific peripherals and hardware pin assignments
// provided by build-tagged board files (e.g. board_feather-m0.go,
// power_hypnos.go). main.go consumes this struct without importing
// machine, keeping all pin/bus mappings in the board file.
type Board struct {
	MCU hal.MCU

	Serial io.Writer

	LED hal.LED

	I2C struct {
		Bus hal.I2C
		SDA hal.Pin
		SCL hal.Pin
	}

	SPI struct {
		Bus hal.SPI
		SCK hal.Pin
		SDO hal.Pin
		SDI hal.Pin
	}

	ADCPin hal.Pin

	RTCWakePin hal.Pin

	SDCSPins []hal.Pin
}

// ConfigureLED reassigns the board LED to a different GPIO pin.
// Called from main after loading config when the user specifies a
// non-default LED pin.
func (b *Board) ConfigureLED(pin hal.Pin) {
	b.LED = newLED(machine.Pin(pin))
}

type LED struct {
	pin machine.Pin
}

func newLED(pin machine.Pin) *LED {
	l := &LED{pin: pin}
	l.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	return l
}

func (l *LED) On() { l.pin.High() }

func (l *LED) Off() { l.pin.Low() }
