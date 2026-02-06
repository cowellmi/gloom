package main

import (
	"github.com/cowellmi/gloom/internal/hardware"
	"github.com/cowellmi/gloom/internal/log"
)

func main() {
	man, err := NewManager()
	if err != nil {
		println("fatal:", err)
		return
	}

	for {
		reason := man.Sleep()

		switch reason {
		case hardware.WakeSample:
			sample(man)
		case hardware.WakeHeartbeat:
			heartbeat(man)
		}

		man.LogMem()
	}
}

func sample(man *Manager) {
	for _, s := range man.config.Sensors {
		if err := s.Init(); err != nil {
			man.Log(log.LevelError, "failed to initialize: "+s.Name()+": "+err.Error())
			continue
		}

		ms, err := s.Measure()
		if err != nil {
			man.Log(log.LevelError, "failed to measure: "+s.Name()+": "+err.Error())
			continue
		}

		for _, m := range ms {
			man.Log(log.LevelInfo, s.Name()+": "+m.Label+": "+m.Value+" "+m.Unit)
		}
	}
}

func heartbeat(man *Manager) {
	man.Log(log.LevelDebug, "heartbeat")
	// TODO: transmit keep-alive message
}
