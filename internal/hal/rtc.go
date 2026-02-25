package hal

import "time"

// RTC abstracts a real-time clock that can read the current time and
// schedule an alarm interrupt. Implementations wrap chip-specific
// drivers (DS3231, PCF8523, etc.).
type RTC interface {
	// Identifier returns a short human-readable name for the RTC
	// chip (e.g. "DS3231", "PCF8523").
	Identifier() string

	// ReadTime returns the current time from the RTC.
	ReadTime() (time.Time, error)

	// SetAlarm schedules an alarm interrupt at the given target time.
	SetAlarm(target time.Time) error

	// ClearAlarm clears any pending alarm so the interrupt pin releases.
	ClearAlarm() error
}
