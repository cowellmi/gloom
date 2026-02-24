package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sensor"
)

// queue is an outbound Blues Notefile (.qo) that appends one Note per writeMap call.
type queue struct {
	nc   notecard.Requester
	name string
}

func (q *queue) writeMap(body map[string]any) error {
	return q.nc.Request(map[string]any{
		"req":  "note.add",
		"file": q.name,
		"body": body,
	})
}

// NotehubSink implements sensor.Recorder and log.Sink by sending structured
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
func NewNotehubSink(nc notecard.Requester, dataName, logName string) *NotehubSink {
	var s NotehubSink
	if dataName != "" {
		s.data = &queue{nc: nc, name: dataName}
	}
	if logName != "" {
		s.logf = &queue{nc: nc, name: logName}
	}
	return &s
}

func (s *NotehubSink) Record(t time.Time, id string, readings []sensor.Reading) error {
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

func (s *NotehubSink) WriteLog(t time.Time, level log.Level, msg string) error {
	if s.logf == nil {
		return nil
	}
	body := map[string]any{
		"ts":    formatISO(t),
		"level": notehubLevel(level),
		"msg":   msg,
	}
	return s.logf.writeMap(body)
}

func (s *NotehubSink) WriteBytes(t time.Time, level log.Level, msg []byte) error {
	return s.WriteLog(t, level, string(msg))
}

func (s *NotehubSink) Flush() error { return nil }

func formatISO(t time.Time) string {
	var buf [20]byte
	return string(appendTimestamp(buf[:0], t))
}

func notehubLevel(l log.Level) string {
	switch l {
	case log.LevelDebug:
		return "DBG"
	case log.LevelInfo:
		return "INF"
	case log.LevelWarn:
		return "WRN"
	case log.LevelError:
		return "ERR"
	default:
		return "???"
	}
}
