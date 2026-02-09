// Package file implements an output driver that writes sensor
// measurements and log entries to file-like io.Writers. The caller
// provides the writers (e.g. files opened on an SD card FAT
// filesystem) and a sync function called on Flush to ensure data is
// durable before power-down.
//
// Implements log.Sink (for log entries) and sensor.Recorder (for
// measurement batches).
package file

import (
	"io"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Sink writes measurements as CSV to a data writer and log entries as
// text to a log writer. Self-disables each writer independently on
// write error.
type Sink struct {
	data io.Writer
	log  io.Writer
	sync func() error
	name string
}

// New creates a file Sink. data receives CSV measurement rows, log
// receives text log lines, and sync is called on Flush to ensure
// durability. Any parameter may be nil (that output is skipped).
func New(name string, data, log io.Writer, sync func() error) *Sink {
	return &Sink{
		name: name,
		data: data,
		log:  log,
		sync: sync,
	}
}

func (s *Sink) Name() string { return s.name }

// Record formats each measurement as a CSV row:
//
//	timestamp,device,label,value,unit
func (s *Sink) Record(buf []byte, t time.Time, device string, ms []sensor.Measurement) error {
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

func (s *Sink) Flush() error {
	if s.sync != nil {
		return s.sync()
	}
	return nil
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
