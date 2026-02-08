package fake

import (
	"math/rand/v2"
	"strconv"

	"github.com/cowellmi/gloom/internal/sensor"
)

type Device struct{}

func NewDevice() *Device {
	return &Device{}
}

func (*Device) Init() error {
	return nil
}

func (*Device) Name() string {
	return "fake"
}

func (*Device) Measure() ([]sensor.Measurement, error) {
	n := rand.Int()
	m := sensor.Measurement{
		Label: "foo",
		Value: strconv.Itoa(n),
		Unit:  "bars",
	}

	return []sensor.Measurement{m}, nil
}
