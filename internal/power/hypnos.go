// Package power provides hal.PowerManager implementations for boards
// with MOSFET-switched power rails.
package power

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

const (
	// powerOnDelay is how long to wait after enabling 5V rails for
	// voltages to stabilise.
	powerOnDelay = 2 * time.Second

	// sdPowerCycleDelay is how long to hold rails off during the
	// initial power cycle so the SD card's internal capacitors
	// discharge and its SPI state machine resets. 250 ms is
	// conservative; most cards discharge in under 100 ms.
	sdPowerCycleDelay = 250 * time.Millisecond
)

// Hypnos implements hal.PowerManager for the OPEnS Lab Hypnos board.
// It controls two MOSFET-switched rails:
//   - 3.3V rail (active-low on rails3Pin)
//   - 5V rail   (active-high on rails5Pin)
type Hypnos struct {
	rails3 machine.Pin
	rails5 machine.Pin
}

// compile-time check
var _ hal.PowerManager = (*Hypnos)(nil)

// NewHypnos creates a Hypnos power manager using the standard D5/D6
// rail pins. It configures the pins as outputs and performs an initial
// power cycle to reset peripherals (SD card SPI state, etc.).
func NewHypnos() *Hypnos {
	h := &Hypnos{
		rails3: machine.D5,
		rails5: machine.D6,
	}

	h.rails3.Configure(machine.PinConfig{Mode: machine.PinOutput})
	h.rails5.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Force a clean power cycle so peripherals reset cleanly.
	h.PowerOff()
	wait.For(sdPowerCycleDelay)
	h.PowerOn()
	wait.For(powerOnDelay)

	return h
}

// PowerOn enables both 3.3V and 5V rails.
func (h *Hypnos) PowerOn() {
	h.rails3.Low()  // active-low
	h.rails5.High() // active-high
}

// PowerOff disables both 3.3V and 5V rails.
func (h *Hypnos) PowerOff() {
	h.rails3.High()
	h.rails5.Low()
}

// Delay returns how long to wait after PowerOn for voltages to
// stabilise before talking to peripherals.
func (h *Hypnos) Delay() time.Duration {
	return powerOnDelay
}
