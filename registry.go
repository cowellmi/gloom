package main

import (
	"github.com/cowellmi/gloom/internal/sensors"
	"github.com/cowellmi/gloom/internal/sensors/fake"
)

var sensorRegistry = map[string]func() sensors.Device{
	"fake": func() sensors.Device { return fake.NewDevice() },
}
