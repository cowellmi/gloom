package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/sensor"
)

type Sink interface {
	Flush() error
}

type LogSink interface {
	Sink
	Log(t time.Time, level config.LogLevel, msg string) error
}

type DataSink interface {
	Sink
	Data(t time.Time, id string, readings []sensor.Reading) error
}
