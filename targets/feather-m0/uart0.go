package main

import (
	"device/sam"
	"machine"
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
