package notehub

import (
	"time"

	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sensor"
)

type Sink struct {
	nc *notecard.Device
}

func New(nc *notecard.Device) (*Sink, error) {
	s := &Sink{nc: nc}
	return s, nil
}

func (s *Sink) Record(t time.Time, id string, readings []sensor.Reading) error {
	debug.Log("Record: " + t.String())
	return nil
}

func (s *Sink) WriteLog(t time.Time, level log.Level, msg string) error {
	debug.Log("WriteLog: " + msg)
	return nil
}

func (s *Sink) WriteBytes(t time.Time, level log.Level, msg []byte) error {
	debug.Log("WriteBytes: " + string(msg))
	return nil
}

func (s *Sink) Flush() error {
	debug.Log("Flushed.")
	return nil
}
