package hypnos

import (
	"machine"
)

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
)

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn() {
	Rail3V.Low() // Hypnos 3.3V rail is active-low
	Rail5V.High()

	// Using waitForRTC instead.
	//time.Sleep(time.Second) // Give time for rails to turn on
}

func powerOff() {
	Rail3V.High()
	Rail5V.Low()
}
