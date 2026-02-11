package hypnos

import (
	"machine"
	"time"
)

const (
	Rail3V            = machine.D5
	Rail5V            = machine.D6
	PowerOnRailsDelay = 2 * time.Second
)

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn() {
	Rail3V.Low() // Hypnos 3.3V rail is active-low
	Rail5V.High()

	// This is required until we can develop a better way
	// for determining if the system has power reilably.
	time.Sleep(PowerOnRailsDelay)
}

func powerOff() {
	Rail3V.High()
	Rail5V.Low()
}
