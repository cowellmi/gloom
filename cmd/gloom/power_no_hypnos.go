//go:build feather_m0 && no_hypnos

package main

import "github.com/cowellmi/gloom/internal/hal"

// initRails returns nil when the Hypnos FeatherWing is not attached.
// No power rail code is compiled into the binary.
func initRails() hal.Rails { return nil }
