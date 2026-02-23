// Package sleeper provides the hardware sleep/wake platform for Gloom.
//
// Sleeper composes an MCU, optional RTC, and optional power rails into
// a single Sleep method. Deadline tracking and fired-slot resolution
// live in the manager package; this package is pure hardware mechanics.
package sleeper

import (
	"errors"
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

	// idleFiredPins holds pins detected as active during idle-sleep
	// polling (when Standby was not entered). Read by PinFired.
	// Reset at the start of each idle-sleep entry.
	idleFiredPins []hal.Pin
}

// New creates a Device. mcu is required. rtc and rails may be nil.
// If an RTC is provided its wake pin is registered automatically.
func New(mcu hal.MCU, rtc hal.RTC, rails hal.Rails) *Device {
	s := &Device{
		mcu:           mcu,
		rtc:           rtc,
		rails:         rails,
		idleFiredPins: make([]hal.Pin, 0, 4),
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

// readTime returns the current time from the RTC, falling back to
// time.Now() if no RTC is attached or if the read fails.
func (s *Device) readTime() (time.Time, error) {
	if s.rtc != nil {
		t, err := s.rtc.ReadTime()
		if err != nil {
			// Falling back to system time and reporting error.
			return time.Now(), err
		}
		return t, nil
	} else {
		// No RTC; using system time.
		return time.Now(), nil
	}
}

// PinFired reports whether the given pin fired in the most recent
// sleep cycle — either via the Standby interrupt-flag snapshot or
// via idle-sleep polling (PinActive). Uses uint8 so the caller
// (manager) does not need to import hal.
func (s *Device) PinFired(pin uint8) bool {
	p := hal.Pin(pin)
	for _, fp := range s.idleFiredPins {
		if fp == p {
			return true
		}
	}
	return s.mcu.PinFired(p)
}

// PowerOnSensorRails powers on sensor-specific rails (on-demand
// rails). No-op if rails is nil.
func (s *Device) PowerOnSensorRails() {
	if s.rails != nil {
		s.rails.Power(hal.RailsFull)
	}
}

// Sleep enters deep or idle sleep until target, then returns the
// actual wake time from the RTC (or time.Now() on read failure).
// Any RTC read errors encountered during the cycle are joined and
// returned so the caller can log them as warnings.
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
	var errs []error
	var remaining time.Duration
	shouldSleep := target.IsZero()
	if !target.IsZero() {
		now, err := s.readTime()
		errs = append(errs, err)
		remaining = target.Sub(now)
		// If readTime failed, now may be a bogus system time that makes
		// remaining negative. Still sleep: the RTC alarm uses the absolute
		// target and will fire correctly regardless of the time calculation.
		shouldSleep = err != nil || remaining > 0
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
				// deepSleep may have cut rails before failing. Restore core
				// rails so idleSleep can reach the RTC via I2C.
				if s.rails != nil {
					s.rails.Power(hal.RailsCore)
				}
				errs = append(errs, s.idleSleep(target))
			}
		} else {
			errs = append(errs, s.idleSleep(target))
		}

		s.mcu.PetWatchdog()

		// Restore core rails so the RTC and SD card are reachable.
		if s.rails != nil {
			s.rails.Power(hal.RailsCore)
		}

		if s.rtc != nil {
			_ = s.rtc.ClearWake() // best-effort
		}

		s.mcu.PetWatchdog()
	}

	wakeTime, err := s.readTime()
	errs = append(errs, err)
	return wakeTime, errors.Join(errs...)
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
		s.rails.Power(hal.RailsOff)
	}

	for i, pin := range s.wakePins {
		if err := s.mcu.ArmWake(pin); err != nil {
			for j := 0; j < i; j++ {
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
// between each, until target is reached. Returns all RTC read errors
// joined, or nil. The tick duration is chosen so the watchdog is
// petted well within its ~8s timeout window.
//
// A zero target means external-interrupt-only: arm wake pins and
// poll PinActive each tick until at least one pin is asserted. The
// fired pins are recorded in idleFiredPins so PinFired returns true
// for them. This is the fallback path when deepSleep fails; all wake
// sources are level-sensitive (active-low) so polling is reliable.
//
// For indefinite or long waits, sensor rails are powered off to
// save power while keeping core rails up for RTC/SD access.
func (s *Device) idleSleep(target time.Time) error {
	const tick = 4 * time.Second

	if target.IsZero() {
		if s.rails != nil {
			s.rails.Power(hal.RailsCore)
		}
		// Arm pins so PinInputPullup is configured and we can poll.
		for _, pin := range s.wakePins {
			_ = s.mcu.ArmWake(pin)
		}
		s.idleFiredPins = s.idleFiredPins[:0]
		for {
			s.mcu.PetWatchdog()
			wait.For(tick)
			for _, pin := range s.wakePins {
				if s.mcu.PinActive(pin) {
					s.idleFiredPins = append(s.idleFiredPins, pin)
				}
			}
			if len(s.idleFiredPins) > 0 {
				for _, pin := range s.wakePins {
					s.mcu.DisarmWake(pin)
				}
				return nil
			}
		}
	}

	// For timed waits, cut on-demand rails if the remaining time is
	// long enough to justify the power savings.
	var errs []error
	now, err := s.readTime()
	errs = append(errs, err)
	if target.Sub(now) > minDeepSleep {
		if s.rails != nil {
			s.rails.Power(hal.RailsCore)
		}
	}

	for now.Before(target) {
		s.mcu.PetWatchdog()
		remaining := min(target.Sub(now), tick)
		wait.For(remaining)
		now, err = s.readTime()
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
