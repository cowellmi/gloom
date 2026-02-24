//go:build tinygo

package led

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

// LED is an output pin driven as an LED.
type LED hal.Pin

// New configures pin as a digital output.
func New(pin hal.Pin) LED {
	machine.Pin(pin).Configure(machine.PinConfig{Mode: machine.PinOutput})
	return LED(pin)
}

func (p LED) Pin() hal.Pin { return hal.Pin(p) }

func (p LED) On() {
	machine.Pin(p).High()
}

func (p LED) Off() {
	machine.Pin(p).Low()
}

func (p LED) Blink() {
	p.On()
	wait.For(50 * time.Millisecond)
	p.Off()
	wait.For(100 * time.Millisecond)
	p.On()
	wait.For(50 * time.Millisecond)
	p.Off()
}
