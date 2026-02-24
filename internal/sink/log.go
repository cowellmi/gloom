package sink

import (
	"io"
	"time"

	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/log"
)

// LogSink implements log.Sink, writing one text line per entry to an
// io.Writer:
//
//	timestamp LEVEL msg
type LogSink struct {
	w   io.Writer
	buf [256]byte
}

// NewLogSink wraps w as a LogSink.
func NewLogSink(w io.Writer) *LogSink {
	return &LogSink{w: w}
}

func (s *LogSink) WriteLog(t time.Time, level log.Level, msg string) error {
	b := s.buf[:0]
	b = appendTimestamp(b, t)
	b = fmtbuf.AppendByte(b, ' ')
	b = log.AppendLevel(b, level)
	b = fmtbuf.AppendByte(b, ' ')
	b = fmtbuf.Append(b, msg)
	b = fmtbuf.AppendByte(b, '\n')
	_, err := s.w.Write(b)
	return err
}

func (s *LogSink) WriteBytes(t time.Time, level log.Level, msg []byte) error {
	b := s.buf[:0]
	b = appendTimestamp(b, t)
	b = fmtbuf.AppendByte(b, ' ')
	b = log.AppendLevel(b, level)
	b = fmtbuf.AppendByte(b, ' ')
	b = fmtbuf.AppendBytes(b, msg)
	b = fmtbuf.AppendByte(b, '\n')
	_, err := s.w.Write(b)
	return err
}

func (s *LogSink) Flush() error { return nil }
