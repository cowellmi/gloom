package sensor

type Measurement struct {
	Label string
	Value string
	Unit  string
}

type Sensor interface {
	Name() string
	Measure() ([]Measurement, error)
}
