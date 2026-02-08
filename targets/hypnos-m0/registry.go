package main

import (
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/fake"
)

var sensorRegistry = map[string]func() sensor.Device{
	"fake": func() sensor.Device { return fake.NewDevice() },
}
