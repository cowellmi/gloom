package hypnos

import "machine"

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
)

func Configure() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func PowerUp() {
	Rail3V.Low()  // Hypnos 3.3V is Active-Low
	Rail5V.High() // Hypnos 5V is Active-High
}

func PowerDown() {
	Rail3V.High()
	Rail5V.Low()
}
