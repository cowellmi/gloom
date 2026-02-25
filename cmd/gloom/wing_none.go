//go:build feather_m0 && no_hypnos

package main

import (
	"errors"

	"github.com/cowellmi/gloom/internal/fallback"
	"github.com/cowellmi/gloom/internal/hal"
)

// No wing attached; use fallbacks.
func initWing() Wing {
	return Wing{
		Rails: fallback.Rails{},
	}
}

func (Wing) ProbeRTC(hal.I2C) (hal.Clock, error) {
	return nil, errors.New("no rtc present")
}
