//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// Hypnos
func initDevices() Devices {
	return Devices{
		InterruptPins:    []hal.Pin{hal.Pin(machine.D12)},
		SDChipSelectPins: []hal.Pin{hal.Pin(machine.D11), hal.Pin(machine.D10)},
	}
}
