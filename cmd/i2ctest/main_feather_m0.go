//go:build feather_m0

package main

import (
	"device/sam"
	"io"
	"machine"
	"strconv"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
)

func initMCU() (hal.MCU, io.Writer) {
	uart0 := machine.UART0
	uart0.Configure(machine.UARTConfig{
		TX: machine.UART0_TX_PIN,
		RX: machine.UART0_RX_PIN,
	})

	proc := samd21.New()
	proc.EnableWatchdog()
	return proc, io.MultiWriter(uart0, machine.Serial)
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
