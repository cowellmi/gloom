package sensors

import (
	"math/rand/v2"
	"strconv"
)

type Fake struct{}

func (f *Fake) Name() string {
	return "fake"
}

func (f *Fake) Measure() ([]Measurement, error) {
	n := rand.Int()
	m := Measurement{
		Label: "foo",
		Value: strconv.Itoa(n),
		Unit:  "bars",
	}

	return []Measurement{m}, nil
}
