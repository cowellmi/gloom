package led

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
)

type LED struct {
	pin hal.Pin
}

func NewLED(pin hal.Pin) *LED {
	return &LED{pin: pin}
}

func (l *LED) On()  { machine.Pin(l.pin).High() }
func (l *LED) Off() { machine.Pin(l.pin).Low() }

// Configure sets pin as a digital output. Call before NewLED or before
// reassigning board.LED at runtime.
func Configure(pin hal.Pin) {
	p := machine.Pin(pin)
	p.Configure(machine.PinConfig{Mode: machine.PinOutput})
}
