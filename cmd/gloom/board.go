//go:build tinygo

package main

import (
	"io"

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
		Bus  hal.I2C
		TxFn func(addr uint16, wb []byte, rb []byte) (err error)
		SDA  hal.Pin
		SCL  hal.Pin
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

	Sensors []string
}
