package platform

import "time"

type WakeReason uint8

const (
	WakeSample    WakeReason = 1 << 0
	WakeHeartbeat WakeReason = 1 << 1
)

type System interface {
	Name() string
	ReadFile(name string) ([]byte, error)
	ReadTime() (time.Time, error)
	Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error)
}
