//go:build feather_m0

package main

import (
	"machine"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/targets/samd21"
)

// initBoard configures the Feather M0 (SAMD21) and returns a Board
// with all peripherals ready. Board-specific pin defaults are applied
// to cfg before returning so they can be overridden by config.ini.
func initBoard(cfg *config.Config) Board {
	var board Board

	// UART serial.
	uart0 := machine.UART0
	_ = uart0.Configure(machine.UARTConfig{
		TX: machine.UART0_TX_PIN,
		RX: machine.UART0_RX_PIN,
	})
	board.UART = uart0

	// USB-CDC serial.
	_ = machine.Serial.Configure(machine.UARTConfig{BaudRate: 115200})
	board.USBCDC = machine.Serial

	// MCU.
	board.MCU = samd21.New()

	// I2C.
	board.I2C = machine.I2C0
	board.SDA = uint8(machine.SDA_PIN)
	board.SCL = uint8(machine.SCL_PIN)

	// SPI.
	board.SPI.Bus = machine.SPI0
	board.SPI.SCK = uint8(machine.SPI0_SCK_PIN)
	board.SPI.SDO = uint8(machine.SPI0_SDO_PIN)
	board.SPI.SDI = uint8(machine.SPI0_SDI_PIN)

	// Board-specific config defaults.
	cfg.Device.SDCSPins = []uint8{uint8(machine.D11), uint8(machine.D10)}
	cfg.Device.RTCWakePin = uint8(machine.D12)
	cfg.Device.LedPin = uint8(machine.LED)

	return board
}
