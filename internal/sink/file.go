// Package sink provides output drivers for sensor data and log entries.
//
// RotaryFileSink writes to daily-rotating files on an SD card (or any
// filesystem exposed via an Opener). SerialSink and NotehubSink write
// to serial and Blues Notehub respectively.
package sink

import (
	"errors"
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
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
// RotaryFileSink generates filenames as "{Dir}/{YYYYMMDD}{Ext}",
// e.g. "GLOOM/20260214.CSV".
type FileSpec struct {
	Dir string
	Ext string
}

// --- internal formatters ---

// csvWriter formats sensor readings as CSV lines to an io.Writer.
type csvWriter struct {
	w   io.Writer
	buf [256]byte
}

func newCSVWriter(w io.Writer) *csvWriter { return &csvWriter{w: w} }

func (r *csvWriter) record(t time.Time, id string, readings []sensor.Reading) error {
	for _, rd := range readings {
		b := r.buf[:0]
		b = appendTimestamp(b, t)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, id)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, rd.Label)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.AppendInt(b, int64(rd.Value), 10)
		b = fmtbuf.AppendByte(b, ',')
		b = fmtbuf.Append(b, rd.Unit)
		b = fmtbuf.AppendByte(b, '\n')
		if _, err := r.w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// logWriter formats log entries as text lines to an io.Writer.
type logWriter struct {
	w   io.Writer
	buf [256]byte
}

func newLogWriter(w io.Writer) *logWriter { return &logWriter{w: w} }

func (w *logWriter) write(t time.Time, level config.LogLevel, msg string) error {
	b := w.buf[:0]
	b = appendTimestamp(b, t)
	b = fmtbuf.AppendByte(b, ' ')
	b = fmtbuf.AppendLevel(b, level)
	b = fmtbuf.AppendByte(b, ' ')
	b = fmtbuf.Append(b, msg)
	b = fmtbuf.AppendByte(b, '\n')
	_, err := w.w.Write(b)
	return err
}

// --- RotaryFileSink ---

// RotaryFileSink writes sensor data as CSV and log entries as text to
// daily-rotating files. Each stream self-disables on a write error.
type RotaryFileSink struct {
	open     Opener
	dataSpec FileSpec
	logSpec  FileSpec
	dataFile AppendFile
	logFile  AppendFile
	dataRec  *csvWriter
	logRec   *logWriter
	curDate  uint32 // YYYYMMDD — zero means no file open yet
}

// NewRotaryFileSink creates a RotaryFileSink with daily-rotating files.
// data and logSpec specify the naming patterns for sensor data and log
// output respectively. An empty Dir in either spec disables that stream.
//
// The sink opens its initial files eagerly using now as the starting date.
// Open errors are returned but the sink is still usable — the failing
// stream is disabled while the other continues.
func NewRotaryFileSink(opener Opener, data, logSpec FileSpec, now time.Time) (*RotaryFileSink, error) {
	s := &RotaryFileSink{
		open:     opener,
		dataSpec: data,
		logSpec:  logSpec,
	}
	err := s.openForDate(now)
	return s, err
}

// Data formats each reading as a CSV row and writes it to the data file.
func (s *RotaryFileSink) Data(t time.Time, id string, readings []sensor.Reading) error {
	if s.dataRec == nil {
		return nil
	}
	if err := s.maybeRotate(t); err != nil {
		return err
	}
	if err := s.dataRec.record(t, id, readings); err != nil {
		s.dataFile = nil
		s.dataRec = nil
		return err
	}
	return nil
}

// Log formats a log line and writes it to the log file.
func (s *RotaryFileSink) Log(t time.Time, level config.LogLevel, msg string) error {
	if s.logRec == nil {
		return nil
	}
	if err := s.maybeRotate(t); err != nil {
		return err
	}
	if err := s.logRec.write(t, level, msg); err != nil {
		s.logFile = nil
		s.logRec = nil
		return err
	}
	return nil
}

// Flush syncs both open files to durable storage.
func (s *RotaryFileSink) Flush() error {
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

func (s *RotaryFileSink) maybeRotate(t time.Time) error {
	if dateKey(t) == s.curDate {
		return nil
	}
	return s.openForDate(t)
}

func (s *RotaryFileSink) openForDate(t time.Time) error {
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
			s.dataRec = newCSVWriter(f)
		}
	}

	if s.logSpec.Dir != "" {
		f, err := s.open(buildFilename(s.logSpec, t))
		if err != nil {
			errs = append(errs, err)
		} else {
			s.logFile = f
			s.logRec = newLogWriter(f)
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
	b := make([]byte, 0, len(spec.Dir)+1+8+len(spec.Ext))
	b = append(b, spec.Dir...)
	b = append(b, '/')
	b = append4(b, y)
	b = append2(b, int(m))
	b = append2(b, d)
	b = append(b, spec.Ext...)
	return string(b)
}

// --- shared ISO timestamp helper (used by csvWriter and NotehubSink) ---

func appendTimestamp(buf []byte, t time.Time) []byte {
	y, mon, d := t.Date()
	h, min, sec := t.Clock()
	buf = append4(buf, y)
	buf = fmtbuf.AppendByte(buf, '-')
	buf = append2(buf, int(mon))
	buf = fmtbuf.AppendByte(buf, '-')
	buf = append2(buf, d)
	buf = fmtbuf.AppendByte(buf, 'T')
	buf = append2(buf, h)
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, min)
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, sec)
	return buf
}

func append2(buf []byte, n int) []byte {
	return append(buf, byte('0'+n/10), byte('0'+n%10))
}

func append4(buf []byte, n int) []byte {
	return append(buf,
		byte('0'+n/1000),
		byte('0'+(n/100)%10),
		byte('0'+(n/10)%10),
		byte('0'+n%10),
	)
}
