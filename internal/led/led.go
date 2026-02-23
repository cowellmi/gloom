//go:build tinygo

package led

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

// LED is an output pin driven as an LED.
type LED hal.Pin

// New configures pin as a digital output.
func New(pin hal.Pin) LED {
	machine.Pin(pin).Configure(machine.PinConfig{Mode: machine.PinOutput})
	return LED(pin)
}

func (p LED) On() {
	machine.Pin(p).High()
}

func (p LED) Off() {
	machine.Pin(p).Low()
}
