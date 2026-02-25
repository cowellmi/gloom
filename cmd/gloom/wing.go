package main

import (
	"github.com/cowellmi/gloom/internal/hal"
)

// A Wing (like a FeatherWing) provides hardware features.
type Wing struct {
	RTCInterruptPin  hal.Pin
	SDChipSelectPins []hal.Pin
	Rails            hal.Rails
}
