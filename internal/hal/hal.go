// Package hal defines the hardware abstraction layer for Gloom.
package hal

import (
	"time"
)

type WakeReason uint8

const (
	WakeExternal WakeReason = iota
	WakeSample
	WakeHeartbeat
)

// Platform abstracts the hardware a board provides: a clock, sleep
// modes, and an identifier. Storage and other optional capabilities
// are expressed as separate interfaces.
type Platform interface {
	Identifier() string
	ReadTime() (time.Time, error)
	Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error)
}

// Fallback is the degraded-mode Platform used when the primary board
// (e.g. Hypnos) fails to probe. Clock uses time.Now(); sleep uses
// time.Sleep; no storage or RTC alarms.
type Fallback struct{}

func (*Fallback) Identifier() string { return "Fallback" }

func (*Fallback) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

// Sleep uses time.Sleep for the sample interval. Heartbeat is ignored:
// only platforms with RTC alarms support it. The absence of heartbeat
// messages upstream signals that the device needs a physical check.
func (*Fallback) Sleep(sampleInterval, _ time.Duration) (WakeReason, error) {
	time.Sleep(sampleInterval)
	return WakeSample, nil
}
