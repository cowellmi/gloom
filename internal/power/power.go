// Package power provides a generic hal.Rails implementation for
// boards with MOSFET-switched power rails. Each rail is a GPIO pin
// with a configurable polarity and a WakeReason bitmask that controls
// when it activates.
package power

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// Rail describes a single MOSFET-switched power rail.
type Rail struct {
	pin       machine.Pin
	activeLow bool
	wakeFor   hal.WakeReason // bitmask: rail activates when wakeFor & reason != 0
}

// NewRail creates a Rail. activeLow inverts the pin logic (Low = on).
// wakeFor is a WakeReason bitmask — use hal.WakeAlways for core rails
// that must be on every wake, or hal.WakeSample for sensor rails.
func NewRail(pin machine.Pin, activeLow bool, wakeFor hal.WakeReason) Rail {
	return Rail{pin: pin, activeLow: activeLow, wakeFor: wakeFor}
}

func (r Rail) on() {
	if r.activeLow {
		r.pin.Low()
	} else {
		r.pin.High()
	}
}

func (r Rail) off() {
	if r.activeLow {
		r.pin.High()
	} else {
		r.pin.Low()
	}
}

// Controller controls one or more MOSFET-switched power rails.
// It satisfies hal.Rails.
type Controller struct {
	rails []Rail
}

// compile-time check
var _ hal.Rails = (*Controller)(nil)

func NewController(rails ...Rail) *Controller {
	m := &Controller{rails: rails}

	for _, r := range m.rails {
		r.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	return m
}

// PowerOn enables all rails whose wakeOn mask overlaps with reason.
// Rails that don't match are left unchanged.
func (m *Controller) PowerOn(reason hal.WakeReason) {
	for _, r := range m.rails {
		if r.wakeFor&reason != 0 {
			r.on()
		}
	}
}

// PowerOff disables all power rails.
func (m *Controller) PowerOff() {
	for _, r := range m.rails {
		r.off()
	}
}
