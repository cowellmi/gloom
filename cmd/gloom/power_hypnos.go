//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"
)

// initRails returns the Hypnos FeatherWing power rail defaults:
//   - D5: 3.3V core rail (active-low), always on after wake so the
//     RTC and SD card are reachable.
//   - D6: 5V sensor rail (active-high), only powered when at least
//     one fired group has sensors.
//   - 250ms sensor delay for MOSFET switching and sensor power-on.
func initRails() ([]RailConfig, time.Duration) {
	return []RailConfig{
		{Name: "3v3", Pin: uint8(machine.D5), ActiveLow: true, Always: true},
		{Name: "5v", Pin: uint8(machine.D6), ActiveLow: false, Always: false},
	}, 250 * time.Millisecond
}
