// Package serial implements a Sink that writes human-readable text
// lines to an io.Writer (typically machine.Serial).
package serial

import (
	"io"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink writes formatted text lines to a serial io.Writer.
// Self-disables on write error: if a write fails, the writer is
// set to nil and all subsequent calls become no-ops.
type Sink struct {
	w io.Writer
}

// New creates a serial Sink. If w is nil, all writes are no-ops.
func New(w io.Writer) *Sink {
	return &Sink{w: w}
}

func (*Sink) Name() string { return "serial" }

func (s *Sink) WriteMeasurements(buf []byte, t time.Time, device string, ms []sensor.Measurement) error {
	if s.w == nil {
		return nil
	}
	for _, m := range ms {
		buf = buf[:0]
		buf = appendTimestamp(buf, t)
		buf = append(buf, "INF | "...)
		buf = append(buf, device...)
		buf = append(buf, ": "...)
		buf = append(buf, m.Label...)
		buf = append(buf, ": "...)
		buf = append(buf, m.Value...)
		buf = append(buf, ' ')
		buf = append(buf, m.Unit...)
		buf = append(buf, '\n')
		if _, err := s.w.Write(buf); err != nil {
			s.w = nil
			return err
		}
	}
	return nil
}

func (s *Sink) WriteLog(buf []byte, t time.Time, level log.Level, msg string) error {
	if s.w == nil {
		return nil
	}
	buf = buf[:0]
	buf = appendTimestamp(buf, t)
	buf = appendLevel(buf, level)
	buf = append(buf, " | "...)
	buf = append(buf, msg...)
	buf = append(buf, '\n')
	if _, err := s.w.Write(buf); err != nil {
		s.w = nil
		return err
	}
	return nil
}

func (*Sink) Flush() error { return nil }

func appendTimestamp(buf []byte, t time.Time) []byte {
	buf = append(buf, '[')
	buf = appendTwoDigits(buf, t.Hour())
	buf = append(buf, ':')
	buf = appendTwoDigits(buf, t.Minute())
	buf = append(buf, ':')
	buf = appendTwoDigits(buf, t.Second())
	buf = append(buf, ']', ' ')
	return buf
}

func appendTwoDigits(buf []byte, n int) []byte {
	if n < 10 {
		buf = append(buf, '0')
	}
	return strconv.AppendInt(buf, int64(n), 10)
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
		return append(buf, "???"...)
	}
}
