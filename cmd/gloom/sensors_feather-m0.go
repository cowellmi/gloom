//go:build feather_m0

package main

import (
	"machine"

	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/battery"
)

// initSensors registers board-level sensors available on the
// Feather M0 hardware (independent of FeatherWing attachments).
func initSensors() {
	sensorRegistry["vbat"] = func() sensor.Device {
		return battery.NewDevice(uint8(machine.D9))
	}
}
