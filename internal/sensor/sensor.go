package sensor

import "time"

// Measurement holds a single sensor reading.
type Measurement struct {
	Label string
	Value string
	Unit  string
}

// Device abstracts a sensor that produces measurements.
type Device interface {
	Init() error
	Name() string
	Measure() ([]Measurement, error)
}

// Recorder receives measurement batches for output to a destination
// (SD card, serial, network, LoRa). Implementations decide their own
// serialization format (CSV, text, JSON, etc) and manage their own
// scratch buffers internally to avoid per-call heap allocations.
type Recorder interface {
	Name() string
	Record(t time.Time, device string, ms []Measurement) error
	Flush() error
}
