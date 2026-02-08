// Package sink defines the Sink interface for outputting sensor
// measurements and log entries to multiple destinations (serial, SD
// card, network). The manager fans out data to all registered sinks.
//
// Design notes:
//
// The manager owns the sink list and iterates over it. A failing sink
// must not block others. Sinks self-disable on persistent write errors
// (e.g. serial disconnected, SD card pulled) so subsequent calls
// become no-ops rather than repeatedly failing.
//
// Flush is called before sleep so that buffered SD writes hit the FAT
// and network sinks transmit their payloads before the MCU enters
// standby and powers down peripherals.
package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink receives measurement batches and log entries for output.
// Implementations decide their own serialization format.
type Sink interface {
	// Name identifies this sink for diagnostics (e.g. "sd", "serial", "notecard").
	Name() string

	// WriteMeasurements writes a batch of sensor measurements taken at
	// time t from the named device. Implementations choose format: CSV
	// to SD, JSON to Notecard, etc.
	WriteMeasurements(t time.Time, device string, ms []sensor.Measurement) error

	// WriteLog writes a single log entry. Implementations may ignore
	// logs entirely (e.g. a LoRa sink might only send measurements).
	WriteLog(t time.Time, level log.Level, msg string) error

	// Flush forces any buffered data to be written. Called by the
	// manager before entering sleep so nothing is lost across a
	// power-down cycle.
	Flush() error
}
