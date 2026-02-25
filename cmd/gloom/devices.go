package main

import "github.com/cowellmi/gloom/internal/hal"

type Devices struct {
	InterruptPins    []hal.Pin
	SDChipSelectPins []hal.Pin
}
