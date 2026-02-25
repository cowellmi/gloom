//go:build feather_m0

package main

import (
	"io"
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
)

// initBoard configures the Feather M0 (SAMD21) and returns a Board
// with all peripherals and hardware pin assignments ready.
func initBoard() Board {
	var board Board

	board.MCU = samd21.New()

	_ = machine.UART0.Configure(machine.UARTConfig{
		BaudRate: 115200,
		TX:       machine.UART0_TX_PIN,
		RX:       machine.UART0_RX_PIN,
	})
	_ = machine.Serial.Configure(machine.UARTConfig{BaudRate: 115200})
	board.Serial = io.MultiWriter(machine.UART0, machine.Serial)

	board.I2C = machine.I2C0
	board.SDA = hal.Pin(machine.SDA_PIN)
	board.SCL = hal.Pin(machine.SCL_PIN)

	board.SPI = machine.SPI0
	board.SCK = hal.Pin(machine.SPI0_SCK_PIN)
	board.SDO = hal.Pin(machine.SPI0_SDO_PIN)
	board.SDI = hal.Pin(machine.SPI0_SDI_PIN)

	board.ADCPin = hal.Pin(machine.D9)
	board.LEDPin = hal.Pin(machine.LED)

	board.Sensors = []string{"vbat"}

	return board
}
