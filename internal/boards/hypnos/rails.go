package hypnos

import (
	"machine"
)

const (
	Rail33V = machine.D5
	Rail5V  = machine.D6
)

func configureRails() {
	Rail33V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn33() {
	Rail33V.Low() // Hypnos 3.3V rail is active-low
}

func powerOff33() {
	Rail33V.High()
}

func powerOn5() {
	Rail5V.High()
}

func powerOff5() {
	Rail5V.Low()
}
