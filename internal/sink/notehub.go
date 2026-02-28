package sink

import (
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sensor"
)

// queue is an outbound Blues Notefile (.qo) that appends one Note per write call.
type queue struct {
	nc   *notecard.Client
	name string
}

func (q *queue) write(body []byte) error {
	var b []byte
	b = append(b, `{"req":"note.add","file":`...)
	b = strconv.AppendQuote(b, q.name)
	b = append(b, `,"body":`...)
	b = append(b, body...)
	b = append(b, `,"sync":true}`...)
	_, err := q.nc.Do(b)
	return err
}

// NotehubSink implements DataSink and log.Sink by sending structured
// JSON Notes to Blues Notefiles. Each sensor reading and each log entry
// becomes one Note with queryable fields in Notehub:
//
//	Sensor Note (data.qo):  {"ts":"...", "sensor":"vbat", "label":"voltage", "value":3800, "unit":"mV"}
//	Log Note    (log.qo):   {"ts":"...", "level":"ERR", "msg":"rtc: timed out"}
//
// Either stream may be disabled by passing an empty name to NewNotehubSink.
type NotehubSink struct {
	data *queue
	logf *queue
}

// NewNotehubSink creates a NotehubSink backed by two outbound Notefile queues.
// Pass an empty string to disable that output stream.
func NewNotehubSink(nc *notecard.Client, dataName, logName string) *NotehubSink {
	var s NotehubSink
	if dataName != "" {
		s.data = &queue{nc: nc, name: dataName}
	}
	if logName != "" {
		s.logf = &queue{nc: nc, name: logName}
	}
	return &s
}

func (s *NotehubSink) Data(t time.Time, id string, readings []sensor.Reading) error {
	if s.data == nil {
		return nil
	}
	for _, r := range readings {
		var b []byte
		b = append(b, `{"ts":`...)
		b = strconv.AppendQuote(b, formatISO(t))
		b = append(b, `,"sensor":`...)
		b = strconv.AppendQuote(b, id)
		b = append(b, `,"label":`...)
		b = strconv.AppendQuote(b, r.Label)
		b = append(b, `,"value":`...)
		b = strconv.AppendInt(b, int64(r.Value), 10)
		b = append(b, `,"unit":`...)
		b = strconv.AppendQuote(b, r.Unit)
		b = append(b, '}')
		if err := s.data.write(b); err != nil {
			return err
		}
	}
	return nil
}

func (s *NotehubSink) Log(t time.Time, level config.LogLevel, msg string) error {
	if s.logf == nil {
		return nil
	}
	var b []byte
	b = append(b, `{"ts":`...)
	b = strconv.AppendQuote(b, formatISO(t))
	b = append(b, `,"level":`...)
	b = strconv.AppendQuote(b, level.String())
	b = append(b, `,"msg":`...)
	b = strconv.AppendQuote(b, msg)
	b = append(b, '}')
	return s.logf.write(b)
}

func (s *NotehubSink) Flush() error { return nil }

func formatISO(t time.Time) string {
	var buf [20]byte
	return string(fmtbuf.AppendTimestamp(buf[:0], t))
}
