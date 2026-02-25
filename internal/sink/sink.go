package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/sensor"
)

// LogSink receives log entries for output. Implementations decide
// their own serialization format and manage their own scratch buffers
// internally. Flush forces any buffered data to be written (called
// before sleep).
type LogSink interface {
	Log(t time.Time, level config.LogLevel, msg string) error
	Flush() error
}

// DataSink receives measurement batches for output to a destination
// (SD card, serial, network, LoRa). Implementations decide their own
// serialization format (CSV, text, JSON, etc) and manage their own
// scratch buffers internally to avoid per-call heap allocations.
type DataSink interface {
	Data(t time.Time, id string, readings []sensor.Reading) error
	Flush() error
}
