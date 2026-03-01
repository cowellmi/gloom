package sink

import (
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sensor"
)

type NotehubSink struct {
	nc       *notecard.Client
	dataFile string
	logFile  string
	buf      [256]byte
}

func NewNotehubSink(nc *notecard.Client, dataFile, logFile string) *NotehubSink {
	var s NotehubSink
	s.dataFile = dataFile
	s.logFile = logFile
	return &s
}

func (s *NotehubSink) Data(t time.Time, id string, readings []sensor.Reading) error {
	if s.dataFile == "" {
		return nil
	}
	for _, r := range readings {
		b := s.buf[:0]
		b = append(b, `{"req":"note.add","file":`...)
		b = strconv.AppendQuote(b, s.dataFile)
		b = append(b, `,"body":{"ts":"`...)
		b = fmtbuf.AppendTimestamp(b, t)
		b = append(b, `","sensor":`...)
		b = strconv.AppendQuote(b, id)
		b = append(b, `,"label":`...)
		b = strconv.AppendQuote(b, r.Label)
		b = append(b, `,"value":`...)
		b = strconv.AppendInt(b, int64(r.Value), 10)
		b = append(b, `,"unit":`...)
		b = strconv.AppendQuote(b, r.Unit)
		b = append(b, `},"sync":true}`...)
		if _, err := s.nc.Do(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *NotehubSink) Log(t time.Time, level config.LogLevel, msg string) error {
	if s.logFile == "" {
		return nil
	}
	b := s.buf[:0]
	b = append(b, `{"req":"note.add","file":`...)
	b = strconv.AppendQuote(b, s.logFile)
	b = append(b, `,"body":{"ts":"`...)
	b = fmtbuf.AppendTimestamp(b, t)
	b = append(b, `","level":`...)
	b = strconv.AppendQuote(b, level.String())
	b = append(b, `,"msg":`...)
	b = strconv.AppendQuote(b, msg)
	b = append(b, `},"sync":true}`...)
	_, err := s.nc.Do(b)
	return err
}

func (s *NotehubSink) Flush() error { return nil }
