//go:build feather_m0

package main

import (
	"device/sam"
	"machine"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/mcu"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
)

// UART0 on SERCOM0 using D1 (PA10, PAD2) as TX and D0 (PA11, PAD3)
// as RX. The Feather M0 board file only exposes UART1 on SERCOM1
// (D10/D11), but those pins are needed for Hypnos SD card CS.
//
// RX interrupts are NOT enabled because TinyGo's machine init
// registers sercomUSART0 as the IRQ_SERCOM0 handler at compile time
// and we cannot override it. TX works without interrupts (polled).
var uart0buf machine.RingBuffer
var UART0 = &machine.UART{
	Buffer: &uart0buf,
	Bus:    sam.SERCOM0_USART,
	SERCOM: 0,
}

// initMCU configures the Feather M0 (SAMD21): sets up UART0 on
// SERCOM0 for debug output, creates the MCU instance, and enables
// the hardware watchdog. Returns the MCU interface for use by the
// generic boot logic in main.go.
func initMCU() mcu.MCU {
	configureUART0(115200)
	debug.W = UART0

	proc := samd21.New()
	proc.EnableWatchdog()
	return proc
}

// boardDefaults applies Feather M0 pin candidates to the config
// before external configuration is loaded. Only MCU-specific pin
// numbers are set here — FeatherWing-specific presets (like Hypnos
// rails) are resolved in main.go from the "power" config key.
func boardDefaults(cfg *config.Config) {
	// SD card CS pins to probe, in order. All pins are checked so
	// multiple cards can be discovered for redundancy.
	//   D11 — Hypnos v3.3
	//   D10 — Hypnos v3.2, Adalogger FeatherWing
	cfg.SDCSPins = []uint8{uint8(machine.D11), uint8(machine.D10)}

	// DS3231 alarm/interrupt pin on D12 (standard Hypnos wiring).
	cfg.RTCWakePin = uint8(machine.D12)
}

// debugWriter returns the UART used for early debug output. Called by
// main.go to create serial sinks when serial output is enabled.
func debugWriter() *machine.UART {
	return UART0
}

// configureUART0 manually sets up SERCOM0 as a 115200-8N1 UART.
// This mirrors machine.UART.Configure but omits the RX interrupt
// enable to avoid conflicting with the compile-time IRQ_SERCOM0
// handler registered by the machine package init.
func configureUART0(baudRate uint32) {
	// Pin mux: PA10/PA11 → SERCOM0 primary function.
	machine.D1.Configure(machine.PinConfig{Mode: machine.PinSERCOM}) // TX, PAD2
	machine.D0.Configure(machine.PinConfig{Mode: machine.PinSERCOM}) // RX, PAD3

	bus := sam.SERCOM0_USART

	// Reset SERCOM0.
	bus.CTRLA.SetBits(sam.SERCOM_USART_CTRLA_SWRST)
	for bus.CTRLA.HasBits(sam.SERCOM_USART_CTRLA_SWRST) ||
		bus.SYNCBUSY.HasBits(sam.SERCOM_USART_SYNCBUSY_SWRST) {
	}

	// Internal clock, 16x oversampling.
	bus.CTRLA.Set(
		(sam.SERCOM_USART_CTRLA_MODE_USART_INT_CLK << sam.SERCOM_USART_CTRLA_MODE_Pos) |
			(1 << sam.SERCOM_USART_CTRLA_SAMPR_Pos))

	// Baud rate.
	UART0.SetBaudRate(baudRate)

	// Frame format: no parity, LSB first.
	bus.CTRLA.SetBits(
		(0 << sam.SERCOM_USART_CTRLA_FORM_Pos) |
			(1 << sam.SERCOM_USART_CTRLA_DORD_Pos))

	// 8 data bits, 1 stop bit, no parity.
	bus.CTRLB.SetBits(
		(0 << sam.SERCOM_USART_CTRLB_CHSIZE_Pos) |
			(0 << sam.SERCOM_USART_CTRLB_SBMODE_Pos) |
			(0 << sam.SERCOM_USART_CTRLB_PMODE_Pos))

	// Pad mapping (SAMD21 datasheet tables 25-8, 25-9):
	//   TXPO=1 → TX on PAD2 (PA10 / D1)
	//   RXPO=3 → RX on PAD3 (PA11 / D0)
	bus.CTRLA.SetBits(
		(1 << sam.SERCOM_USART_CTRLA_TXPO_Pos) |
			(3 << sam.SERCOM_USART_CTRLA_RXPO_Pos))

	// Enable transmitter and receiver.
	bus.CTRLB.SetBits(sam.SERCOM_USART_CTRLB_TXEN | sam.SERCOM_USART_CTRLB_RXEN)

	// Enable SERCOM0 USART.
	bus.CTRLA.SetBits(sam.SERCOM_USART_CTRLA_ENABLE)
	for bus.SYNCBUSY.HasBits(sam.SERCOM_USART_SYNCBUSY_ENABLE) {
	}
}
