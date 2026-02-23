//go:build feather_m0

package main

import (
	"io"
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/targets/samd21"
)

// initBoard configures the Feather M0 (SAMD21) and returns a Board
// with all peripherals and hardware pin assignments ready.
func initBoard() Board {
	var board Board

	// MCU
	board.MCU = samd21.New()

	// Serial
	_ = machine.UART0.Configure(machine.UARTConfig{
		BaudRate: 115200,
		TX:       machine.UART0_TX_PIN,
		RX:       machine.UART0_RX_PIN,
	})
	_ = machine.Serial.Configure(machine.UARTConfig{BaudRate: 115200})
	board.Serial = io.MultiWriter(machine.UART0, machine.Serial)

	// LED
	board.LED = newLED(machine.LED)

	// I2C
	board.I2C.Bus = machine.I2C0
	board.I2C.SDA = hal.Pin(machine.SDA_PIN)
	board.I2C.SCL = hal.Pin(machine.SCL_PIN)

	// SPI
	board.SPI.Bus = machine.SPI0
	board.SPI.SCK = hal.Pin(machine.SPI0_SCK_PIN)
	board.SPI.SDO = hal.Pin(machine.SPI0_SDO_PIN)
	board.SPI.SDI = hal.Pin(machine.SPI0_SDI_PIN)

	// ADC
	board.ADCPin = hal.Pin(machine.D9)

	// Hypnos
	board.SDCSPins = []hal.Pin{hal.Pin(machine.D11), hal.Pin(machine.D10)}
	board.RTCWakePin = hal.Pin(machine.D12)

	return board
}
