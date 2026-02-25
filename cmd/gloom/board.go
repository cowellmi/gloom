package main

import (
	"io"

	"github.com/cowellmi/gloom/internal/hal"
)

type Board struct {
	MCU hal.MCU

	Serial io.Writer

	I2C hal.I2C
	SDA hal.Pin
	SCL hal.Pin

	SPI hal.SPI
	SCK hal.Pin
	SDO hal.Pin
	SDI hal.Pin

	ADCPin hal.Pin
	LEDPin hal.Pin

	Sensors []string
}
