package sink

import (
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/sensor"
)

// SerialSink writes formatted text lines to a serial io.Writer.
// Write errors are ignored: serial is a diagnostic channel and
// transient failures (e.g. USB CDC host not listening) should not
// impact behaviour.
type SerialSink struct {
	w   io.Writer
	buf [256]byte
}

// NewSerial creates a SerialSink. If w is nil, all writes are no-ops.
func NewSerial(w io.Writer) *SerialSink {
	return &SerialSink{w: w}
}

func (s *SerialSink) Data(t time.Time, id string, readings []sensor.Reading) error {
	if s.w == nil {
		return nil
	}
	for _, r := range readings {
		b := s.buf[:0]
		b = appendSerialTimestamp(b, t)
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

func (s *SerialSink) Log(t time.Time, level config.LogLevel, msg string) error {
	if s.w == nil {
		return nil
	}
	b := s.buf[:0]
	b = appendSerialTimestamp(b, t)
	b = fmtbuf.AppendLevel(b, level)
	b = fmtbuf.Append(b, " | ")
	b = fmtbuf.Append(b, msg)
	b = fmtbuf.AppendByte(b, '\r')
	b = fmtbuf.AppendByte(b, '\n')
	_, _ = s.w.Write(b) // best-effort
	return nil
}

func (*SerialSink) Flush() error { return nil }

func appendSerialTimestamp(buf []byte, t time.Time) []byte {
	buf = fmtbuf.AppendByte(buf, '[')
	buf = append2(buf, t.Hour())
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, t.Minute())
	buf = fmtbuf.AppendByte(buf, ':')
	buf = append2(buf, t.Second())
	buf = fmtbuf.AppendByte(buf, ']')
	buf = fmtbuf.AppendByte(buf, ' ')
	return buf
}
