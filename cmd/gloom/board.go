package main

import (
	"io"
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// Board holds board-specific peripherals and hardware pin assignments
// provided by build-tagged board files (e.g. main_feather-m0.go,
// power_hypnos.go). main.go consumes this struct without importing
// machine, keeping all pin/bus mappings in the board file.
type Board struct {
	MCU hal.MCU

	Serial io.Writer

	LED hal.LED

	I2C struct {
		Bus hal.I2C
		SDA uint8
		SCL uint8
	}

	SPI struct {
		Bus hal.SPI
		SCK uint8
		SDO uint8
		SDI uint8
	}

	ADCPin uint8

	RTCWakePin uint8

	SDCSPins []uint8
}

type LED struct {
	pin machine.Pin
}

func newLED(pin machine.Pin) *LED {
	l := &LED{}
	l.Configure(uint8(pin))
	return l
}

func (l *LED) Configure(pin uint8) {
	l.pin = machine.Pin(pin)
	l.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func (l *LED) On() { l.pin.High() }

func (l *LED) Off() { l.pin.Low() }
