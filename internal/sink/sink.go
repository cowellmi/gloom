package sink

import (
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/sensor"
)

type LogSink interface {
	Log(t time.Time, level config.LogLevel, msg string) error
	Flush() error
}

type DataSink interface {
	Data(t time.Time, id string, readings []sensor.Reading) error
	Flush() error
}
