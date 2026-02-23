// Package sleeper provides the hardware sleep/wake platform for Gloom.
//
// Sleeper composes an MCU, optional RTC, and optional power rails into
// a single Sleep method. Deadline tracking and fired-slot resolution
// live in the manager package; this package is pure hardware mechanics.
package sleeper

import (
	"slices"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

// minDeepSleep is the minimum time remaining before a target deadline
// for deep sleep to be worthwhile. Below this threshold the overhead
// of alarm setup risks the alarm firing before Standby, and idle
// sleep is cheaper anyway.
const minDeepSleep = 2 * time.Second

// Device composes optional hardware components into a unified
// sleep/wake platform. The processor is always required. RTC and
// Rails are optional — pass nil for graceful degradation:
//   - nil RTC: use time.Now(), no alarm-based wake
//   - nil Rails: no rail control
//
// Deadline tracking and fired-slot resolution are the caller's
// responsibility (see internal/manager).
type Device struct {
	mcu        hal.MCU
	rtc        hal.RTC
	rails      hal.Rails
	wakePins   []hal.Pin
	hasExtPins bool // true when any non-RTC wake pin has been added
}

// New creates a Device. mcu is required. rtc and rails may be nil.
// If an RTC is provided its wake pin is registered automatically.
func New(mcu hal.MCU, rtc hal.RTC, rails hal.Rails) *Device {
	s := &Device{
		mcu:   mcu,
		rtc:   rtc,
		rails: rails,
	}
	if rtc != nil {
		s.wakePins = append(s.wakePins, rtc.WakePin())
		// RTC pin is not an ext interrupt — don't set hasExtPins.
	}
	return s
}

// AddWakePin adds a GPIO interrupt pin as a deep-sleep wake source.
// Sets hasExtPins = true. Deduplicates automatically.
func (s *Device) AddWakePin(pin hal.Pin) {
	if !slices.Contains(s.wakePins, pin) {
		s.wakePins = append(s.wakePins, pin)
		s.hasExtPins = true
	}
}

// ReadTime returns the current time from the RTC, or time.Now() if
// no RTC is attached.
func (s *Device) ReadTime() (time.Time, error) {
	if s.rtc != nil {
		return s.rtc.ReadTime()
	}
	return time.Now(), nil
}

// PinFired reports whether the given pin's interrupt flag was set
// when the MCU last woke from Standby. Uses uint8 so the caller
// (manager) does not need to import hal.
func (s *Device) PinFired(pin uint8) bool {
	return s.mcu.PinFired(hal.Pin(pin))
}

// PowerOnSensorRails powers on sensor-specific rails (on-demand
// rails). No-op if rails is nil.
func (s *Device) PowerOnSensorRails() {
	if s.rails != nil {
		s.rails.PowerOn(true)
	}
}

// Sleep enters deep or idle sleep until target, then returns the
// actual wake time from the RTC (or time.Now() if no RTC).
//
// target == zero means external-interrupt-only: no RTC alarm is set.
// If target is non-zero but already in the past, Sleep returns the
// current time immediately without entering hardware sleep.
func (s *Device) Sleep(target time.Time) (time.Time, error) {
	s.mcu.PetWatchdog()

	// Only enter hardware sleep if there is time remaining, or this
	// is an external-interrupt-only configuration (target is zero).
	// Use ReadTime (not wall clock) so the RTC is the authoritative
	// time source for the remaining-time calculation.
	var remaining time.Duration
	shouldSleep := target.IsZero()
	if !target.IsZero() {
		// TODO: short explaination of skip err
		now, _ := s.ReadTime()
		remaining = target.Sub(now)
		shouldSleep = remaining > 0
	}

	if shouldSleep {
		s.mcu.PetWatchdog()

		// Deep sleep requires wake pins and either:
		//   a) a timed target with enough remaining time for an RTC alarm, or
		//   b) an external-interrupt-only configuration (no target, ext pins present).
		canDeepSleep := len(s.wakePins) > 0 &&
			((!target.IsZero() && s.rtc != nil && remaining > minDeepSleep) ||
				(target.IsZero() && s.hasExtPins))

		if canDeepSleep {
			// deepSleep only errors during setup (ClearWake/SetWake/ArmWake),
			// before Standby is called. Fall back to idleSleep for the full
			// remaining interval using the same absolute target.
			if err := s.deepSleep(target); err != nil {
				s.idleSleep(target)
			}
		} else {
			s.idleSleep(target)
		}

		s.mcu.PetWatchdog()

		// Restore always-rails so the RTC and SD card are reachable.
		if s.rails != nil {
			s.rails.PowerOn(false)
		}

		if s.rtc != nil {
			_ = s.rtc.ClearWake() // best-effort
		}

		s.mcu.PetWatchdog()
	}

	return s.ReadTime() // actual wake time
}

// deepSleep sets the RTC alarm (if present), arms all wake pins,
// cuts power, and enters MCU standby.
func (s *Device) deepSleep(target time.Time) error {
	if s.rtc != nil && !target.IsZero() {
		if err := s.rtc.ClearWake(); err != nil {
			return err
		}
		if err := s.rtc.SetWake(target); err != nil {
			return err
		}
	}

	s.mcu.PetWatchdog()

	if s.rails != nil {
		s.rails.PowerOff()
	}

	for i, pin := range s.wakePins {
		if err := s.mcu.ArmWake(pin); err != nil {
			for j := 0; j <= i; j++ {
				s.mcu.DisarmWake(s.wakePins[j])
			}
			return err
		}
	}

	s.mcu.DisableWatchdog()
	s.mcu.Standby()

	for _, pin := range s.wakePins {
		s.mcu.DisarmWake(pin)
	}

	s.mcu.EnableWatchdog()
	s.mcu.PetWatchdog()

	return nil
}

// idleSleep busy-waits in short intervals, petting the watchdog
// between each, until target is reached. The tick duration is chosen
// so the watchdog is petted well within its ~8s timeout window.
// A zero target means external-interrupt-only: loop indefinitely.
func (s *Device) idleSleep(target time.Time) {
	const tick = 4 * time.Second

	if target.IsZero() {
		for {
			s.mcu.PetWatchdog()
			wait.For(tick)
		}
	}

	for time.Now().Before(target) {
		s.mcu.PetWatchdog()
		remaining := min(time.Until(target), tick)
		wait.For(remaining)
	}
}
