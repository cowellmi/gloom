// Package power provides a generic hal.Rails implementation for
// boards with MOSFET-switched power rails. Each rail is a GPIO pin
// with a configurable polarity and a WakeReason bitmask that controls
// when it activates.
package power

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

const (
	// StabiliseDelay is how long to wait after enabling rails for
	// voltages to stabilise before talking to peripherals.
	StabiliseDelay = 2 * time.Second

	// powerCycleDelay is how long to hold rails off during the
	// initial power cycle so SD card capacitors discharge and SPI
	// state machines reset.
	powerCycleDelay = 250 * time.Millisecond
)

// Rail describes a single MOSFET-switched power rail.
type Rail struct {
	pin       machine.Pin
	activeLow bool
	wakeFor hal.WakeReason // bitmask: rail activates when wakeFor & reason != 0
}

// NewRail creates a Rail. activeLow inverts the pin logic (Low = on).
// wakeFor is a WakeReason bitmask — use hal.WakeAlways for core rails
// that must be on every wake, or hal.WakeSample for sensor rails.
func NewRail(pin uint8, activeLow bool, wakeFor hal.WakeReason) Rail {
	return Rail{pin: machine.Pin(pin), activeLow: activeLow, wakeFor: wakeFor}
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

// NewController creates a power Controller for the given rails. It
// configures each pin as an output and performs an initial power cycle
// to reset peripherals (SD card SPI state, etc.).
func NewController(rails ...Rail) *Controller {
	m := &Controller{rails: rails}

	for _, r := range m.rails {
		r.pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	}

	// Force a clean power cycle so peripherals reset cleanly.
	m.PowerOff()
	wait.For(powerCycleDelay)
	m.PowerOn(hal.WakeAlways)
	wait.For(StabiliseDelay)

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

// Delay returns how long to wait after PowerOn for voltages to
// stabilise before talking to peripherals.
func (m *Controller) Delay() time.Duration {
	return StabiliseDelay
}
