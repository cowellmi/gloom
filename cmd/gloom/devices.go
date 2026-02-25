package main

import (
	"github.com/cowellmi/gloom/internal/hal"
)

type Devices struct {
	InterruptPins    []hal.Pin
	SDChipSelectPins []hal.Pin
	LED              hal.LED
	RTC              hal.RTC
	NIC              hal.NIC
	Rails            hal.Rails
}
