//go:build feather_m0 && no_hypnos

package main

import (
	"github.com/cowellmi/gloom/internal/fallback"
	"github.com/cowellmi/gloom/internal/hal"
)

// No wing attached; use fallbacks.
func initWing() Wing {
	return Wing{
		RTCInterruptPin: hal.NoPin,
		Rails:           fallback.Rails{},
	}
}

func (Wing) ProbeRTC(hal.I2C) (hal.Clock, error) {
	return fallback.Clock{}, nil
}
