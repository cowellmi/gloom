// Package serial implements an output driver that writes human-readable
// text lines to an io.Writer (typically machine.Serial).
//
// Implements log.Sink (for log entries) and sensor.Recorder (for
// measurement batches).
package serial

import (
	"io"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink writes formatted text lines to a serial io.Writer.
// Write errors are ignored: serial is a diagnostic channel and
// transient failures (e.g. USB-CDC host not listening) should not
// impact behaviour.
type Sink struct {
	w   io.Writer
	buf [128]byte
}

// NewSink creates a serial Sink. If w is nil, all writes are no-ops.
func NewSink(w io.Writer) *Sink {
	return &Sink{w: w}
}

func (*Sink) Name() string { return "serial" }

func (s *Sink) Record(t time.Time, device string, ms []sensor.Measurement) error {
	if s.w == nil {
		return nil
	}
	for _, m := range ms {
		b := s.buf[:0]
		b = appendTimestamp(b, t)
		b = append(b, "INF | "...)
		b = append(b, device...)
		b = append(b, ": "...)
		b = append(b, m.Label...)
		b = append(b, ": "...)
		b = append(b, m.Value...)
		b = append(b, ' ')
		b = append(b, m.Unit...)
		b = append(b, '\r', '\n')
		_, _ = s.w.Write(b) // best-effort
	}
	return nil
}

func (s *Sink) WriteLog(t time.Time, level log.Level, msg string) error {
	if s.w == nil {
		return nil
	}
	b := s.buf[:0]
	b = appendTimestamp(b, t)
	b = log.AppendLevel(b, level)
	b = append(b, " | "...)
	b = append(b, msg...)
	b = append(b, '\r', '\n')
	_, _ = s.w.Write(b) // best-effort
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

