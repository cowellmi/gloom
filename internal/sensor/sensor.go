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
// serialization format (CSV, text, JSON, etc).
//
// buf is scratch space for formatting; implementations append into it
// and write to their destination. The buffer is passed as buf[:0] so
// recorders get the full backing capacity with zero length. This
// avoids per-call heap allocations.
type Recorder interface {
	Name() string
	Record(buf []byte, t time.Time, device string, ms []Measurement) error
	Flush() error
}
