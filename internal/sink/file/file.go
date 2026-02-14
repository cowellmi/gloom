// Package file implements an output driver that writes sensor
// measurements and log entries to daily-rotating files on an SD card
// (or any filesystem exposed via an Opener function).
//
// Each day gets its own pair of files — one for sensor data (CSV) and
// one for log entries (text). When the date changes (detected from the
// timestamp passed into Record/WriteLog), the Sink closes the old
// files and opens new ones with the new date stamp.
//
// Implements log.Sink (for log entries) and sensor.Recorder (for
// measurement batches).
package file

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// AppendFile is a file handle that supports appending, syncing to
// durable storage, and closing. Satisfied by sdcard.File and any
// similar implementation.
type AppendFile interface {
	io.WriteCloser
	Sync() error
}

// Opener opens or creates a named file for append writing.
type Opener func(name string) (AppendFile, error)

// FileSpec describes the naming pattern for a rotating file.
// The Sink generates filenames as "{Dir}/{YYYYMMDD}{Ext}",
// e.g. "data/20260214.csv". The caller is responsible for creating
// Dir on the filesystem before the first write.
type FileSpec struct {
	Dir string // e.g. "data", "logs"
	Ext string // e.g. ".csv", ".log"
}

// Sink writes measurements as CSV to a data file and log entries as
// text to a log file. Files rotate daily. Self-disables each writer
// independently on write error.
type Sink struct {
	name     string
	open     Opener
	dataSpec FileSpec
	logSpec  FileSpec
	data     AppendFile
	log      AppendFile
	curDate  uint32 // YYYYMMDD — zero means no file open yet
}

// New creates a file Sink that uses opener to create daily-rotating
// files. data and log specify the naming patterns for the sensor data
// and log files respectively. An empty Dir in either spec disables
// that output.
//
// The Sink opens its initial files eagerly using now as the starting
// date. Open errors are returned but the Sink is still usable — the
// failing stream is disabled while the other continues to work.
func New(name string, opener Opener, data, logSpec FileSpec, now time.Time) (*Sink, error) {
	s := &Sink{
		name:     name,
		open:     opener,
		dataSpec: data,
		logSpec:  logSpec,
	}
	err := s.openForDate(now)
	return s, err
}

func (s *Sink) Name() string { return s.name }

// Record formats each measurement as a CSV row:
//
//	timestamp,device,label,value,unit
func (s *Sink) Record(buf []byte, t time.Time, device string, ms []sensor.Measurement) error {
	s.maybeRotate(t)
	if s.data == nil {
		return nil
	}
	ts := formatTimestamp(t)
	for _, m := range ms {
		buf = buf[:0]
		buf = append(buf, ts...)
		buf = append(buf, ',')
		buf = append(buf, device...)
		buf = append(buf, ',')
		buf = append(buf, m.Label...)
		buf = append(buf, ',')
		buf = append(buf, m.Value...)
		buf = append(buf, ',')
		buf = append(buf, m.Unit...)
		buf = append(buf, '\n')
		if _, err := s.data.Write(buf); err != nil {
			s.data = nil
			return err
		}
	}
	return nil
}

// WriteLog formats a log line: timestamp level msg
func (s *Sink) WriteLog(buf []byte, t time.Time, level log.Level, msg string) error {
	s.maybeRotate(t)
	if s.log == nil {
		return nil
	}
	buf = buf[:0]
	buf = append(buf, formatTimestamp(t)...)
	buf = append(buf, ' ')
	buf = appendLevel(buf, level)
	buf = append(buf, ' ')
	buf = append(buf, msg...)
	buf = append(buf, '\n')
	if _, err := s.log.Write(buf); err != nil {
		s.log = nil
		return err
	}
	return nil
}

// Flush syncs both open files to durable storage.
func (s *Sink) Flush() error {
	var errs []error
	if s.data != nil {
		if err := s.data.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.log != nil {
		if err := s.log.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// maybeRotate checks whether the date has changed and opens new files
// if so. Called at the top of every write path.
func (s *Sink) maybeRotate(t time.Time) {
	dk := dateKey(t)
	if dk == s.curDate {
		return
	}
	s.openForDate(t)
}

// openForDate closes any existing files and opens new date-stamped
// files. Errors opening individual files disable that stream but do
// not prevent the other from working.
func (s *Sink) openForDate(t time.Time) error {
	var errs []error

	// Close existing files. Sync before close so data written in
	// the current cycle is durable.
	if s.data != nil {
		// Sync errors on close are non-fatal; we're moving on.
		_ = s.data.Sync()
		_ = s.data.Close()
		s.data = nil
	}
	if s.log != nil {
		_ = s.log.Sync()
		_ = s.log.Close()
		s.log = nil
	}

	s.curDate = dateKey(t)

	if s.dataSpec.Dir != "" {
		f, err := s.open(buildFilename(s.dataSpec, t))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.data = f
		}
	}

	if s.logSpec.Dir != "" {
		f, err := s.open(buildFilename(s.logSpec, t))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.log = f
		}
	}

	return errors.Join(errs...)
}

// dateKey returns a uint32 encoding of the date as YYYYMMDD for fast
// comparison.
func dateKey(t time.Time) uint32 {
	y, m, d := t.Date()
	return uint32(y)*10000 + uint32(m)*100 + uint32(d)
}

// buildFilename returns "{Dir}/{YYYYMMDD}{Ext}", e.g.
// "data/20260214.csv".
func buildFilename(spec FileSpec, t time.Time) string {
	y, m, d := t.Date()
	var date [8]byte
	put4(date[:], y)
	put2(date[4:], int(m))
	put2(date[6:], int(d))
	return spec.Dir + "/" + string(date[:]) + spec.Ext
}

// formatTimestamp returns "YYYY-MM-DDTHH:MM:SS" (fixed 19 bytes).
func formatTimestamp(t time.Time) string {
	var buf [19]byte
	y, mon, d := t.Date()
	h, min, sec := t.Clock()
	put4(buf[:], y)
	buf[4] = '-'
	put2(buf[5:], int(mon))
	buf[7] = '-'
	put2(buf[8:], d)
	buf[10] = 'T'
	put2(buf[11:], h)
	buf[13] = ':'
	put2(buf[14:], min)
	buf[16] = ':'
	put2(buf[17:], sec)
	return string(buf[:])
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

func appendLevel(buf []byte, level log.Level) []byte {
	switch level {
	case log.LevelDebug:
		return append(buf, "DBG"...)
	case log.LevelInfo:
		return append(buf, "INF"...)
	case log.LevelWarn:
		return append(buf, "WRN"...)
	case log.LevelError:
		return append(buf, "ERR"...)
	default:
		return strconv.AppendInt(buf, int64(level), 10)
	}
}
