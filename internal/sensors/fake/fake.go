package fake

import (
	"math/rand/v2"
	"strconv"

	"github.com/cowellmi/gloom/internal/sensors"
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

func (*Device) Measure() ([]sensors.Measurement, error) {
	n := rand.Int()
	m := sensors.Measurement{
		Label: "foo",
		Value: strconv.Itoa(n),
		Unit:  "bars",
	}

	return []sensors.Measurement{m}, nil
}
