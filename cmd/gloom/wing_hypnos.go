//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
)

// Hypnos
func initWing() Wing {
	var wing Wing

	wing.InterruptPins = []hal.Pin{hal.Pin(machine.D12)}
	wing.SDChipSelectPins = []hal.Pin{hal.Pin(machine.D11), hal.Pin(machine.D10)}

	wing.Rails = power.NewController(
		power.NewRail(hal.Pin(machine.D5), power.ActiveLow, hal.RailsCore, 0),
		power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, hal.RailsFull, 250*time.Millisecond),
	)

	return wing
}
