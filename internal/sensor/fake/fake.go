package fake

import (
	"math/rand/v2"
	"strconv"

	"github.com/cowellmi/gloom/internal/sensor"
)

type Device struct {
	ms [1]sensor.Measurement
}

func NewDevice() *Device {
	return &Device{}
}

func (*Device) Init() error {
	return nil
}

func (*Device) Name() string {
	return "fake"
}

func (d *Device) Measure() ([]sensor.Measurement, error) {
	n := rand.Int()
	d.ms[0] = sensor.Measurement{
		Label: "foo",
		Value: strconv.Itoa(n),
		Unit:  "bars",
	}
	return d.ms[:], nil
}
