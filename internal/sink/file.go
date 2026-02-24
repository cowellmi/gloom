// Package sink provides output drivers for sensor data and log entries.
//
// FileSink writes to daily-rotating files on an SD card (or any
// filesystem exposed via an Opener). CSVRecorder and LogSink wrap any
// io.Writer with CSV/text formatting. SerialSink writes human-readable
// lines to a serial io.Writer.
package sink

import (
	"errors"
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// AppendFile is a file handle that supports appending, syncing to
// durable storage, and closing. Satisfied by sdcard.File.
type AppendFile interface {
	io.WriteCloser
	Sync() error
}

// Opener opens or creates a named file for append writing.
type Opener func(name string) (AppendFile, error)

// FileSpec describes the naming pattern for a rotating file.
// FileSink generates filenames as "{Dir}/{YYYYMMDD}{Ext}",
// e.g. "GLOOM/20260214.CSV".
type FileSpec struct {
	Dir string
	Ext string
}

// FileSink writes sensor data as CSV and log entries as text to
// daily-rotating files. It delegates format logic to internal
// CSVRecorder and LogSink instances. Each stream self-disables on a
// write error.
type FileSink struct {
	name     string
	open     Opener
	dataSpec FileSpec
	logSpec  FileSpec
	dataFile AppendFile
	logFile  AppendFile
	dataRec  *CSVRecorder
	logRec   *LogSink
	curDate  uint32 // YYYYMMDD — zero means no file open yet
}

// NewFileSink creates a FileSink with daily-rotating files. data and
// logSpec specify the naming patterns for sensor data and log output
// respectively. An empty Dir in either spec disables that output.
//
// The sink opens its initial files eagerly using now as the starting
// date. Open errors are returned but the sink is still usable — the
// failing stream is disabled while the other continues.
func NewFileSink(name string, opener Opener, data, logSpec FileSpec, now time.Time) (*FileSink, error) {
	s := &FileSink{
		name:     name,
		open:     opener,
		dataSpec: data,
		logSpec:  logSpec,
	}
	err := s.openForDate(now)
	return s, err
}

func (s *FileSink) Name() string { return s.name }

// Record formats each reading as a CSV row and writes it to the data file.
func (s *FileSink) Record(t time.Time, id string, readings []sensor.Reading) error {
	if s.dataRec == nil {
		return nil
	}
	if err := s.maybeRotate(t); err != nil {
		return err
	}
	if err := s.dataRec.Record(t, id, readings); err != nil {
		s.dataFile = nil
		s.dataRec = nil
		return err
	}
	return nil
}

// WriteLog formats a log line and writes it to the log file.
func (s *FileSink) WriteLog(t time.Time, level log.Level, msg string) error {
	if s.logRec == nil {
		return nil
	}
	if err := s.maybeRotate(t); err != nil {
		return err
	}
	if err := s.logRec.WriteLog(t, level, msg); err != nil {
		s.logFile = nil
		s.logRec = nil
		return err
	}
	return nil
}

// WriteBytes formats a log line from a byte slice and writes it to the log file.
func (s *FileSink) WriteBytes(t time.Time, level log.Level, msg []byte) error {
	if s.logRec == nil {
		return nil
	}
	if err := s.maybeRotate(t); err != nil {
		return err
	}
	if err := s.logRec.WriteBytes(t, level, msg); err != nil {
		s.logFile = nil
		s.logRec = nil
		return err
	}
	return nil
}

// Flush syncs both open files to durable storage.
func (s *FileSink) Flush() error {
	var errs []error
	if s.dataFile != nil {
		if err := s.dataFile.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.logFile != nil {
		if err := s.logFile.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *FileSink) maybeRotate(t time.Time) error {
	dk := dateKey(t)
	if dk == s.curDate {
		return nil
	}
	return s.openForDate(t)
}

func (s *FileSink) openForDate(t time.Time) error {
	var errs []error

	if s.dataFile != nil {
		_ = s.dataFile.Sync()
		_ = s.dataFile.Close()
		s.dataFile = nil
		s.dataRec = nil
	}
	if s.logFile != nil {
		_ = s.logFile.Sync()
		_ = s.logFile.Close()
		s.logFile = nil
		s.logRec = nil
	}

	s.curDate = dateKey(t)

	if s.dataSpec.Dir != "" {
		f, err := s.open(buildFilename(s.dataSpec, t))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.dataFile = f
			s.dataRec = NewCSVRecorder(f)
		}
	}

	if s.logSpec.Dir != "" {
		f, err := s.open(buildFilename(s.logSpec, t))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.logFile = f
			s.logRec = NewLogSink(f)
		}
	}

	return errors.Join(errs...)
}

func dateKey(t time.Time) uint32 {
	y, m, d := t.Date()
	return uint32(y)*10000 + uint32(m)*100 + uint32(d)
}

func buildFilename(spec FileSpec, t time.Time) string {
	y, m, d := t.Date()
	var date [8]byte
	put4(date[:], y)
	put2(date[4:], int(m))
	put2(date[6:], int(d))
	return spec.Dir + "/" + string(date[:]) + spec.Ext
}

func put2(b []byte, n int) {
	b[0] = byte('0' + n/10)
	b[1] = byte('0' + n%10)
}

func put4(b []byte, n int) {
	b[0] = byte('0' + n/1000)
	b[1] = byte('0' + (n/100)%10)
	b[2] = byte('0' + (n/10)%10)
	b[3] = byte('0' + n%10)
}
