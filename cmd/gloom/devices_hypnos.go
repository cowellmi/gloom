//go:build feather_m0 && !no_hypnos

package main

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
	"github.com/cowellmi/gloom/internal/rtc/ds3231"
	"github.com/cowellmi/gloom/internal/wait"
)

// Hypnos
func initDevices(mcu hal.MCU, bus hal.I2C) (Devices, error) {
	var dev Devices
	var probeErrs []error

	dev.InterruptPins = []hal.Pin{hal.Pin(machine.D12)}
	dev.SDChipSelectPins = []hal.Pin{hal.Pin(machine.D11), hal.Pin(machine.D10)}

	dev.Rails = power.NewController(
		power.NewRail(hal.Pin(machine.D5), power.ActiveLow, hal.RailsCore, 0),
		power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, hal.RailsFull, 250*time.Millisecond),
	)

	// Power-cycle
	dev.Rails.Power(hal.RailsOff)
	wait.For(250 * time.Millisecond)
	mcu.PetWatchdog()
	dev.Rails.Power(hal.RailsCore)
	wait.For(2 * time.Second)
	mcu.PetWatchdog()

	ds, err := ds3231.Probe(bus)
	if err != nil {
		probeErrs = append(probeErrs, err)
	} else {
		dev.RTC = ds
	}
	mcu.PetWatchdog()

	return dev, errors.Join(probeErrs...)
}
