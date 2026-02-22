//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
)

// initRails returns the Hypnos FeatherWing power rail controller:
//
//   - D5: 3.3V core rail (active-low), always on after wake so the
//     RTC and SD card are reachable. Delay 0 — it's already up
//     before sensor rails are needed.
//   - D6: 5V sensor rail (active-high), only powered when at least
//     one fired group has sensors. 250ms delay for MOSFET switching
//     and sensor power-on.
func initRails() hal.Rails {
	debug.Log("init: hypnos power rails")
	return power.NewController(
		power.NewRail(uint8(machine.D5), power.ActiveLow, true, 0),
		power.NewRail(uint8(machine.D6), power.ActiveHigh, false, 250*time.Millisecond),
	)
}
