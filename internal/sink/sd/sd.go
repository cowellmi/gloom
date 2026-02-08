// Package sd will implement a Sink that writes sensor measurements
// and log entries to files on an SD card via the Hypnos board's card
// reader.
//
// Measurements will be written in CSV format (one file per day or
// session) for easy retrieval and analysis. Log entries will be
// written to a separate log file.
//
// The SD card is powered by a MOSFET rail on the Hypnos board, so
// Flush must ensure all buffered data is written to the FAT before
// the manager powers down the rail and enters standby.
//
// TODO: implement using TinyFS or a minimal FAT driver compatible
// with TinyGo on SAMD21.
package sd

import (
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink writes measurements as CSV and logs as text to SD card files.
type Sink struct {
	// TODO: file handles, buffer, FAT driver reference
}

// New creates an SD card Sink.
// TODO: accept a FAT/filesystem handle and base path.
func New() *Sink {
	return &Sink{}
}

func (*Sink) Name() string { return "sd" }

// WriteMeasurements writes measurements in CSV format to the data file.
// TODO: implement CSV formatting and file append.
func (s *Sink) WriteMeasurements(t time.Time, device string, ms []sensor.Measurement) error {
	_ = t
	_ = device
	_ = ms
	return nil // TODO
}

// WriteLog writes a log entry to the log file on SD.
// TODO: implement text formatting and file append.
func (s *Sink) WriteLog(t time.Time, level log.Level, msg string) error {
	_ = t
	_ = level
	_ = msg
	return nil // TODO
}

// Flush syncs all buffered data to the SD card's FAT.
// Must be called before powering down the SD rail.
// TODO: implement FAT sync.
func (s *Sink) Flush() error {
	return nil // TODO
}
