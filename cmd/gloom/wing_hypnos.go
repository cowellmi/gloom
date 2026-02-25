//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
	"github.com/cowellmi/gloom/internal/wait"
)

// Hypnos
func initWing() Wing {
	var wing Wing

	wing.InterruptPins = []hal.Pin{hal.Pin(machine.D12)}
	wing.SDChipSelectPins = []hal.Pin{hal.Pin(machine.D11), hal.Pin(machine.D10)}

	rail3v := power.NewRail(hal.Pin(machine.D5), power.ActiveLow, hal.RailsCore, 0)
	rail5v := power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, hal.RailsFull, 250*time.Millisecond)
	wing.Rails = power.NewController("hypnos", rail3v, rail5v)

	// Power-cycle
	wing.Rails.Power(hal.RailsOff)
	wait.For(250 * time.Millisecond)
	wing.Rails.Power(hal.RailsCore)
	wait.For(2 * time.Second)

	return wing
}
