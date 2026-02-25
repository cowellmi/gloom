package main

import (
	"github.com/cowellmi/gloom/internal/hal"
)

type Wing struct {
	InterruptPins    []hal.Pin
	SDChipSelectPins []hal.Pin
	Rails            hal.Rails
}
