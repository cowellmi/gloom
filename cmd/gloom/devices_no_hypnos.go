//go:build feather_m0 && no_hypnos

package main

import "github.com/cowellmi/gloom/internal/hal"

// initWakePin returns NoPin when Hypnos is not attached.
func initDevices(mcu hal.MCU, bus hal.I2C) (Devices, error) {
	return Devices{}, nil
}
