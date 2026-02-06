package hardware

import "time"

type WakeReason uint8

const (
	WakeSample WakeReason = iota
	WakeHeartbeat
)

type Platform interface {
	Name() string
	ReadFile(name string) ([]byte, error)
	ReadTime() (time.Time, error)

	// Sleep puts the device into a low-power state. The device wakes
	// on whichever event occurs first: the sample interval alarm, the
	// heartbeat interval alarm, or an external sensor interrupt.
	//
	// A zero duration disables that alarm.
	Sleep(sample, heartbeat time.Duration) (WakeReason, error)
}
