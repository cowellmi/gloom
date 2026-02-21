//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
)

// boardPower returns the Hypnos FeatherWing power rails:
//   - D5: 3.3V core rail (active-low), always on after wake so the
//     RTC and SD card are reachable.
//   - D6: 5V sensor rail (active-high), only powered when at least
//     one fired group has sensors.
func boardPower() []power.Rail {
	return []power.Rail{
		power.NewRail(machine.D5, true, hal.WakeAlways),
		power.NewRail(machine.D6, false, hal.WakeSensors),
	}
}
