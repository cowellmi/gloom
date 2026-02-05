package sensor

type Sensor interface {
	Name() string
	Measure() (string, error)
}
