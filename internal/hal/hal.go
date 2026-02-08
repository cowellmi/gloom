// Package hal defines the hardware abstraction layer for Gloom.
// Platform is the core interface that every board must implement.
package hal

import "time"

type WakeReason uint8

const (
	WakeSample    WakeReason = 1 << 0
	WakeHeartbeat WakeReason = 1 << 1
)

// Platform abstracts the hardware a board provides: a clock, sleep
// modes, and an identifier. Storage and other optional capabilities
// are expressed as separate interfaces.
type Platform interface {
	Identifier() string
	ReadTime() (time.Time, error)
	Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error)
}
