//go:build feather_m0

package main

import (
	"device/sam"
	"machine"
	"strconv"

	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/targets/samd21"
)

var uart0buf machine.RingBuffer
var UART0 = &machine.UART{
	Buffer: &uart0buf,
	Bus:    sam.SERCOM0_USART,
	SERCOM: 0,
}

func initMCU() hal.MCU {
	configureUART0(115200)
	debug.W = UART0

	proc := samd21.New()
	proc.EnableWatchdog()
	return proc
}

func resetCause() string {
	r := sam.PM.RCAUSE.Get()
	hex := "0x" + strconv.FormatUint(uint64(r), 16)
	switch {
	case r&(1<<5) != 0:
		return hex + " (WDT)"
	case r&(1<<4) != 0:
		return hex + " (EXT)"
	case r&0x01 != 0:
		return hex + " (POR)"
	case r&(1<<6) != 0:
		return hex + " (SYST)"
	default:
		return hex
	}
}

func isWDTReset() bool {
	return sam.PM.RCAUSE.Get()&(1<<5) != 0
}

// configureUART0 sets up SERCOM0 as 115200-8N1 on D1 (TX) / D0 (RX).
// Mirrors the main app's board file but omits the RX interrupt to
// avoid conflicting with TinyGo's compile-time IRQ_SERCOM0 handler.
func configureUART0(baudRate uint32) {
	machine.D1.Configure(machine.PinConfig{Mode: machine.PinSERCOM})
	machine.D0.Configure(machine.PinConfig{Mode: machine.PinSERCOM})

	bus := sam.SERCOM0_USART

	bus.CTRLA.SetBits(sam.SERCOM_USART_CTRLA_SWRST)
	for bus.CTRLA.HasBits(sam.SERCOM_USART_CTRLA_SWRST) ||
		bus.SYNCBUSY.HasBits(sam.SERCOM_USART_SYNCBUSY_SWRST) {
	}

	bus.CTRLA.Set(
		(sam.SERCOM_USART_CTRLA_MODE_USART_INT_CLK << sam.SERCOM_USART_CTRLA_MODE_Pos) |
			(1 << sam.SERCOM_USART_CTRLA_SAMPR_Pos))

	UART0.SetBaudRate(baudRate)

	bus.CTRLA.SetBits(
		(0 << sam.SERCOM_USART_CTRLA_FORM_Pos) |
			(1 << sam.SERCOM_USART_CTRLA_DORD_Pos))

	bus.CTRLB.SetBits(
		(0 << sam.SERCOM_USART_CTRLB_CHSIZE_Pos) |
			(0 << sam.SERCOM_USART_CTRLB_SBMODE_Pos) |
			(0 << sam.SERCOM_USART_CTRLB_PMODE_Pos))

	bus.CTRLA.SetBits(
		(1 << sam.SERCOM_USART_CTRLA_TXPO_Pos) |
			(3 << sam.SERCOM_USART_CTRLA_RXPO_Pos))

	bus.CTRLB.SetBits(sam.SERCOM_USART_CTRLB_TXEN | sam.SERCOM_USART_CTRLB_RXEN)

	bus.CTRLA.SetBits(sam.SERCOM_USART_CTRLA_ENABLE)
	for bus.SYNCBUSY.HasBits(sam.SERCOM_USART_SYNCBUSY_ENABLE) {
	}
}
