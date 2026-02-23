//go:build tinygo

// Reads voltage from an ADC pin behind a 2:1 voltage divider
// (common on Adafruit Feather boards).
package vbat

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/sensor"
)

type Device struct {
	adc machine.ADC
	ms  [1]sensor.Reading
}

func NewDevice(pin hal.Pin) *Device {
	return &Device{adc: machine.ADC{Pin: machine.Pin(pin)}}
}

func (*Device) ID() string { return "vbat" }

// Measure reads the ADC and returns the battery voltage in millivolts,
// assuming a 2:1 voltage divider with a 3.3V reference:
//
//	mV = raw * 6600 / 65535
func (d *Device) Measure() ([]sensor.Reading, error) {
	d.adc.Configure(machine.ADCConfig{})
	raw := uint32(d.adc.Get())
	d.ms[0] = sensor.Reading{
		Label: "voltage",
		Value: int32((raw * 6600) / 65535),
		Unit:  "mV",
	}
	return d.ms[:], nil
}
