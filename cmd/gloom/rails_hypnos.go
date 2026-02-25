//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
)

// initRails returns the Hypnos FeatherWing power rail controller:
//
//   - D5: 3.3V core rail (active-low), enabled at RailsCore so the
//     RTC and SD card are reachable after every wake. Delay 0.
//   - D6: 5V sensor rail (active-high), enabled at RailsFull only
//     when sensors are active. 250ms delay for MOSFET switching and
//     sensor power-on.
func initRails() hal.Rails {
	return power.NewController(
		power.NewRail(hal.Pin(machine.D5), power.ActiveLow, hal.RailsCore, 0),
		power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, hal.RailsFull, 250*time.Millisecond),
	)
}
