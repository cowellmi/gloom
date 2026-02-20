package main

import (
	"io"

	"tinygo.org/x/drivers"

	"github.com/cowellmi/gloom/internal/hal"
)

// Board holds board-specific peripherals provided by the build-tagged
// board file (e.g. main_feather_m0.go). main.go consumes this struct
// without importing machine, keeping all pin/bus mappings in the board
// file.
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
}
