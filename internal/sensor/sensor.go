package sensor

type Measurement struct {
	Label string
	Value string
	Unit  string
}

type Device interface {
	Init() error
	Name() string
	Measure() ([]Measurement, error)
}
