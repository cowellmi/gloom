//go:build feather_m0

package main

import (
	"machine"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/targets/samd21"
)

// initMCU configures the Feather M0 (SAMD21): sets up UART0 on
// SERCOM0 for debug output, creates the MCU instance, and enables
// the hardware watchdog. Returns the MCU interface for use by the
// generic boot logic in main.go.
func initMCU() hal.MCU {
	uart0 := machine.UART0
	uart0.Configure(machine.UARTConfig{
		TX: machine.UART0_TX_PIN,
		RX: machine.UART0_RX_PIN,
	})
	debug.W = uart0

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
