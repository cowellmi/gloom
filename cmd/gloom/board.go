package main

import (
	"io"
	"time"

	"tinygo.org/x/drivers"

	"github.com/cowellmi/gloom/internal/hal"
)

// RailConfig describes a MOSFET-switched power rail set by board files
// at compile time (not user-configurable via INI).
type RailConfig struct {
	Name      string
	Pin       uint8
	ActiveLow bool
	Always    bool // true = on every wake; false = on-demand (sensors)
}

// Board holds board-specific peripherals and hardware pin assignments
// provided by build-tagged board files (e.g. main_feather_m0.go,
// board_hypnos.go). main.go consumes this struct without importing
// machine, keeping all pin/bus mappings in the board file.
type Board struct {
	MCU hal.MCU
	I2C drivers.I2C
	SDA uint8
	SCL uint8

	SPI struct {
		Bus drivers.SPI
		SCK uint8
		SDO uint8
		SDI uint8
	}

	UART   io.Writer
	USBCDC io.Writer

	LedPin      uint8
	SDCSPins    []uint8
	RTCWakePin  uint8
	Rails       []RailConfig
	SensorDelay time.Duration
}
