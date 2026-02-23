// Package serial implements an output driver that writes human-readable
// text lines to an io.Writer (typically machine.Serial).
//
// Implements log.Sink (for log entries) and sensor.Recorder (for
// measurement batches).
package serial

import (
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink writes formatted text lines to a serial io.Writer.
// Write errors are ignored: serial is a diagnostic channel and
// transient failures (e.g. USB CDC host not listening) should not
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

func (s *Sink) Record(t time.Time, id string, readings []sensor.Reading) error {
	if s.w == nil {
		return nil
	}
	for _, r := range readings {
		b := s.buf[:0]
		b = appendTimestamp(b, t)
		b = fmtbuf.Append(b, "SEN | ")
		b = fmtbuf.Append(b, id)
		b = fmtbuf.Append(b, ": ")
		b = fmtbuf.Append(b, r.Label)
		b = fmtbuf.Append(b, ": ")
		b = fmtbuf.AppendInt(b, int64(r.Value), 10)
		b = fmtbuf.AppendByte(b, ' ')
		b = fmtbuf.Append(b, r.Unit)
		b = fmtbuf.AppendByte(b, '\r')
		b = fmtbuf.AppendByte(b, '\n')
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
	b = fmtbuf.Append(b, " | ")
	b = fmtbuf.Append(b, msg)
	b = fmtbuf.AppendByte(b, '\r')
	b = fmtbuf.AppendByte(b, '\n')
	_, _ = s.w.Write(b) // best-effort
	return nil
}

func (s *Sink) WriteBytes(t time.Time, level log.Level, msg []byte) error {
	if s.w == nil {
		return nil
	}
	b := s.buf[:0]
	b = appendTimestamp(b, t)
	b = log.AppendLevel(b, level)
	b = fmtbuf.Append(b, " | ")
	b = fmtbuf.AppendBytes(b, msg)
	b = fmtbuf.AppendByte(b, '\r')
	b = fmtbuf.AppendByte(b, '\n')
	_, _ = s.w.Write(b) // best-effort
	return nil
}

func (*Sink) Flush() error { return nil }

func appendTimestamp(buf []byte, t time.Time) []byte {
	buf = fmtbuf.AppendByte(buf, '[')
	buf = appendTwoDigits(buf, t.Hour())
	buf = fmtbuf.AppendByte(buf, ':')
	buf = appendTwoDigits(buf, t.Minute())
	buf = fmtbuf.AppendByte(buf, ':')
	buf = appendTwoDigits(buf, t.Second())
	buf = fmtbuf.AppendByte(buf, ']')
	buf = fmtbuf.AppendByte(buf, ' ')
	return buf
}

func appendTwoDigits(buf []byte, n int) []byte {
	if n < 10 {
		buf = fmtbuf.AppendByte(buf, '0')
	}
	return fmtbuf.AppendInt(buf, int64(n), 10)
}
