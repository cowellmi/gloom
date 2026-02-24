package sink

import (
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

type Sink interface {
	sensor.Recorder
	log.Sink
}
