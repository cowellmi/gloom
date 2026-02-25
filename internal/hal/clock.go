package hal

import (
	"time"
)

type Clock interface {
	Identifier() string
	ReadTime() (time.Time, error)
}

type AlarmClock interface {
	Clock

	// SetAlarm schedules an alarm interrupt at the given target time.
	SetAlarm(target time.Time) error

	// ClearAlarm clears any pending alarm so the interrupt pin releases.
	ClearAlarm() error
}
