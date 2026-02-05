package main

import "github.com/cowellmi/gloom/internal/log"

func main() {
	man, err := NewManager()
	if err != nil {
		println("fatal:", err)
		return
	}

	for {
		for _, s := range man.config.Sensors {
			man.Log(log.LevelDebug, s.Name())

			if err := s.Init(); err != nil {
				man.Log(log.LevelError, "failed to initialize: "+s.Name()+": "+err.Error())
			}

			ms, err := s.Measure()
			if err != nil {
				man.Log(log.LevelError, "failed to measure: "+s.Name()+": "+err.Error())
				continue
			}

			for _, m := range ms {
				man.Log(log.LevelDebug, m.Label+": "+m.Value+" "+m.Unit)
			}
		}

		man.sys.Sleep(man.config.SleepInterval)
	}
}
