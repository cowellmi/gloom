//go:build tinygo

// Package power provides a generic hal.Rails implementation for
// boards with MOSFET-switched power rails. Each rail is a GPIO pin
// with a configurable polarity, a minimum activation threshold, and a
// stabilization delay. The controller tracks per-rail on/off state so
// Power only waits for the delay of newly-enabled rails.
package power

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

// Polarity describes the logic level that enables a MOSFET-switched
// power rail.
type Polarity uint8

const (
	ActiveHigh Polarity = iota
	ActiveLow
)

// Rail describes a single MOSFET-switched power rail.
type Rail struct {
	pin       machine.Pin
	polarity  Polarity
	threshold hal.RailState
	delay     time.Duration
}

// NewRail creates a Rail. pin is the GPIO number (converted to
// machine.Pin internally). polarity sets whether the rail is enabled
// by driving the pin High or Low. threshold is the minimum RailState
// at which this rail is enabled — the rail is on when
// Power(state) is called with state >= threshold, and off otherwise.
// delay is the stabilization time to wait after switching this rail on.
func NewRail(pin hal.Pin, polarity Polarity, threshold hal.RailState, delay time.Duration) Rail {
	return Rail{pin: machine.Pin(pin), polarity: polarity, threshold: threshold, delay: delay}
}

func (r Rail) on() {
	if r.polarity == ActiveLow {
		r.pin.Low()
	} else {
		r.pin.High()
	}
}

func (r Rail) off() {
	if r.polarity == ActiveLow {
		r.pin.High()
	} else {
		r.pin.Low()
	}
}

// Controller controls one or more MOSFET-switched power rails.
// It satisfies hal.Rails.
type Controller struct {
	rails []Rail
	on    []bool
}

// compile-time check
var _ hal.Rails = (*Controller)(nil)

// NewController creates a Controller and configures each rail pin as
// an output. All rails start in the off state.
func NewController(rails ...Rail) *Controller {
	m := &Controller{
		rails: rails,
		on:    make([]bool, len(rails)),
	}

	for _, r := range m.rails {
		r.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	return m
}

// Power sets the power rail state. A rail is enabled when
// state >= rail.threshold, and disabled otherwise. Waits for the
// stabilization delay of any newly-enabled rails before returning.
func (m *Controller) Power(state hal.RailState) {
	var maxDelay time.Duration
	for i, r := range m.rails {
		if state >= r.threshold {
			if !m.on[i] {
				r.on()
				m.on[i] = true
				if r.delay > maxDelay {
					maxDelay = r.delay
				}
			}
		} else {
			if m.on[i] {
				r.off()
				m.on[i] = false
			}
		}
	}
	if maxDelay > 0 {
		wait.For(maxDelay)
	}
}
