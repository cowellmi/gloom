package hypnos

import (
	"machine"
)

const (
	rails3Pin = machine.D5
	rails5Pin = machine.D6
)

func configureRails() {
	rails3Pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	rails5Pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func powerOn33() {
	rails3Pin.Low() // Hypnos 3.3V rail is active-low
}

func powerOff33() {
	rails3Pin.High()
}

func powerOn5() {
	rails5Pin.High()
}

func powerOff5() {
	rails5Pin.Low()
}
