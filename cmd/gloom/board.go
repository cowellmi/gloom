package main

import (
	"io"

	"github.com/cowellmi/gloom/internal/hal"
)

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

	Sensors []string
}
