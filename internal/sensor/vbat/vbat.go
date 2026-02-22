// Reads voltage from an ADC pin behind a 2:1 voltage divider
// (common on Adafruit Feather boards).
package vbat

import (
	"machine"
	"strconv"

	"github.com/cowellmi/gloom/internal/sensor"
)

type Device struct {
	adc machine.ADC
	ms  [1]sensor.Measurement
}

func NewDevice(pin uint8) *Device {
	return &Device{adc: machine.ADC{Pin: machine.Pin(pin)}}
}

func (*Device) Init() error  { return nil }
func (*Device) Name() string { return "vbat" }

// Measure reads the ADC and converts the raw 16-bit value to
// voltage assuming a 2:1 divider with a 3.3V reference:
//
//	V = raw * 2 * 3.3 / 65535
//
// The result is formatted to two decimal places using integer math.
func (d *Device) Measure() ([]sensor.Measurement, error) {
	d.adc.Configure(machine.ADCConfig{})
	raw := uint32(d.adc.Get())

	// millivolts = raw * 6600 / 65535
	mv := (raw * 6600) / 65535

	whole := mv / 1000
	frac := (mv % 1000) / 10

	var buf [8]byte
	b := buf[:0]
	b = strconv.AppendUint(b, uint64(whole), 10)
	b = append(b, '.')
	if frac < 10 {
		b = append(b, '0')
	}
	b = strconv.AppendUint(b, uint64(frac), 10)

	d.ms[0] = sensor.Measurement{
		Label: "voltage",
		Value: string(b),
		Unit:  "V",
	}
	return d.ms[:], nil
}
