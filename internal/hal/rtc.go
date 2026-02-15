package hal

import "time"

// RTC abstracts a real-time clock that can read the current time and
// schedule a wake interrupt. Implementations wrap chip-specific
// drivers (DS3231, PCF8523, etc.).
type RTC interface {
	// Identifier returns a short human-readable name for the RTC
	// chip (e.g. "DS3231", "PCF8523").
	Identifier() string

	// ReadTime returns the current time from the RTC.
	ReadTime() (time.Time, error)

	// SetWake schedules a wake interrupt at the given target time.
	// The RTC should assert its interrupt pin when the target is
	// reached.
	SetWake(target time.Time) error

	// ClearWake clears any pending wake interrupt so the interrupt
	// pin releases.
	ClearWake() error

	// WakePin returns the GPIO pin number (as uint8) that the RTC
	// asserts when a wake interrupt fires. Used by the system to
	// configure the MCU wake source.
	WakePin() uint8
}
