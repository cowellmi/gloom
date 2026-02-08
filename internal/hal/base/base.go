package base

import (
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

type System struct{}

func New() *System {
	return &System{}
}

func (*System) Identifier() string { return "Base" }

func (*System) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

// Sleep waits for sampleInterval using a simple time.Sleep.
// The heartbeat parameter is intentionally ignored: Base is the degraded-mode
// fallback used when the primary platform (e.g. Hypnos) fails to probe.
// Heartbeat intervals only make sense on platforms that can deliver them via
// RTC alarms. If a device falls back to Base, the absence of heartbeat
// messages upstream is itself the signal that something is wrong -- prompting
// a physical check of the device and its logs.
func (*System) Sleep(sampleInterval, _ time.Duration) (hal.WakeReason, error) {
	time.Sleep(sampleInterval)
	return hal.WakeSample, nil
}
