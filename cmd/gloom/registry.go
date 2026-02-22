package main

import "github.com/cowellmi/gloom/internal/sensor"

var sensorRegistry = map[string]func() sensor.Device{}
