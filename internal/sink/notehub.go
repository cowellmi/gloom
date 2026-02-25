package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sensor"
)

// queue is an outbound Blues Notefile (.qo) that appends one Note per writeMap call.
type queue struct {
	nc   *notecard.Device
	name string
}

func (q *queue) writeMap(body map[string]any) error {
	return q.nc.Request(map[string]any{
		"req":  "note.add",
		"file": q.name,
		"body": body,
		"sync": true,
	})
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
func NewNotehubSink(nc *notecard.Device, dataName, logName string) *NotehubSink {
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
		body := map[string]any{
			"ts":     formatISO(t),
			"sensor": id,
			"label":  r.Label,
			"value":  r.Value,
			"unit":   r.Unit,
		}
		if err := s.data.writeMap(body); err != nil {
			return err
		}
	}
	return nil
}

func (s *NotehubSink) Log(t time.Time, level config.LogLevel, msg string) error {
	if s.logf == nil {
		return nil
	}
	body := map[string]any{
		"ts":    formatISO(t),
		"level": level.String(),
		"msg":   msg,
	}
	return s.logf.writeMap(body)
}

func (s *NotehubSink) Flush() error { return nil }

func formatISO(t time.Time) string {
	var buf [20]byte
	return string(appendTimestamp(buf[:0], t))
}
