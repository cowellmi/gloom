//go:build feather_m0 && !no_hypnos

package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/power"
	"github.com/cowellmi/gloom/internal/wait"
)

// initRails builds the Hypnos FeatherWing power rails, cycles them
// (off → always-on), and returns the controller. The power-on
// sequence ensures a clean state for the RTC and SD card:
//
//   - D5: 3.3V core rail (active-low), always on after wake so the
//     RTC and SD card are reachable.
//   - D6: 5V sensor rail (active-high), only powered when at least
//     one fired group has sensors.
//   - 250ms sensor delay for MOSFET switching and sensor power-on.
func initRails(pet func()) hal.Rails {
	ctrl := power.NewController(250*time.Millisecond,
		power.NewRail(uint8(machine.D5), true, true),
		power.NewRail(uint8(machine.D6), false, false),
	)
	ctrl.PowerOff()
	wait.For(250 * time.Millisecond)
	ctrl.PowerOn(false)
	pet()
	wait.For(2 * time.Second)
	return ctrl
}
