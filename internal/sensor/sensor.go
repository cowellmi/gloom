package sensor

import "time"

// Reading holds a single sensor measurement with its label, fixed-point
// integer value, and unit. Labels and units are string literals defined
// in the driver (flash, not RAM). Value is fixed-point: scale depends on
// the sensor, e.g. millivolts, millidegrees Celsius, hundredths of a percent.
type Reading struct {
	Label string
	Value int32
	Unit  string
}

// Sensor abstracts a sensor that produces readings. Configure, init, and
// any required startup delay are handled inside Measure — no separate Init
// call is needed from the caller.
type Sensor interface {
	ID() string
	Measure() ([]Reading, error)
}

// Recorder receives measurement batches for output to a destination
// (SD card, serial, network, LoRa). Implementations decide their own
// serialization format (CSV, text, JSON, etc) and manage their own
// scratch buffers internally to avoid per-call heap allocations.
type Recorder interface {
	Name() string
	Record(t time.Time, id string, readings []Reading) error
	Flush() error
}
