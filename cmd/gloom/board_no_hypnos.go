//go:build feather_m0 && no_hypnos

package main

import "github.com/cowellmi/gloom/internal/power"

// boardPower returns nil when the Hypnos FeatherWing is not attached.
// No MOSFET rails are toggled and no rail controller is created.
func boardPower() []power.Rail {
	return nil
}
