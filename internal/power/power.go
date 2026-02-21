// Package power provides a generic hal.Rails implementation for
// boards with MOSFET-switched power rails. Each rail is a GPIO pin
// with a configurable polarity and an always flag that determines
// whether it powers on every wake or only when sensors are needed.
package power

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// Rail describes a single MOSFET-switched power rail.
type Rail struct {
	pin       machine.Pin
	activeLow bool
	always    bool // true = on every wake; false = on-demand (sensors)
}

// NewRail creates a Rail. pin is the GPIO number (converted to
// machine.Pin internally). activeLow inverts the pin logic (Low = on).
// always marks the rail as always-on after wake (core infrastructure
// like RTC and SD card). Non-always rails are on-demand and only
// enabled when fired groups need sensor power.
func NewRail(pin uint8, activeLow bool, always bool) Rail {
	return Rail{pin: machine.Pin(pin), activeLow: activeLow, always: always}
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

// PowerOn enables rails. When sensors is false, only always-rails are
// enabled. When true, all rails (always + on-demand) are enabled.
func (m *Controller) PowerOn(sensors bool) {
	for _, r := range m.rails {
		if r.always || sensors {
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
