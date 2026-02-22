// Package power provides a generic hal.Rails implementation for
// boards with MOSFET-switched power rails. Each rail is a GPIO pin
// with a configurable polarity, an always flag, and a stabilization
// delay. The controller tracks per-rail on/off state so PowerOn only
// waits for the delay of newly-enabled rails.
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
	pin      machine.Pin
	polarity Polarity
	always   bool
	delay    time.Duration
}

// NewRail creates a Rail. pin is the GPIO number (converted to
// machine.Pin internally). polarity sets whether the rail is enabled
// by driving the pin High or Low. always marks the rail as always-on
// after wake (core infrastructure like RTC and SD card). Non-always
// rails are on-demand and only enabled when fired groups need sensor
// power. delay is the stabilization time to wait after switching this
// rail on.
func NewRail(pin hal.Pin, polarity Polarity, always bool, delay time.Duration) Rail {
	return Rail{pin: machine.Pin(pin), polarity: polarity, always: always, delay: delay}
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

// PowerOn enables rails. When sensors is false, only always-rails are
// enabled. When true, all rails (always + on-demand) are enabled.
// After switching, waits for the longest delay among newly-enabled
// rails.
func (m *Controller) PowerOn(sensors bool) {
	var maxDelay time.Duration
	for i, r := range m.rails {
		if (r.always || sensors) && !m.on[i] {
			r.on()
			m.on[i] = true
			if r.delay > maxDelay {
				maxDelay = r.delay
			}
		}
	}
	if maxDelay > 0 {
		wait.For(maxDelay)
	}
}

// PowerOff disables all power rails.
func (m *Controller) PowerOff() {
	for i, r := range m.rails {
		r.off()
		m.on[i] = false
	}
}
