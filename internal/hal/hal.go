// Package hal defines the hardware abstraction layer for Gloom.
//
// The central type is System, a composable struct that assembles
// optional hardware components (RTC, power rails) alongside a
// required MCU into a unified sleep/wake interface. All pin
// references use uint8 so this package remains testable with the
// standard Go toolchain.
package hal

import (
	"errors"
	"slices"
	"time"

	"github.com/cowellmi/gloom/internal/wait"
)

// WakeReason is a bitmask used by power rails to decide which rails
// to enable on wake. WakeAlways matches every reason and is used for
// core infrastructure rails (RTC, SD card). WakeSensors activates
// sensor power rails.
type WakeReason uint8

const (
	WakeSensors  WakeReason = 1 << iota // sensor power rails
	WakeExternal                        // non-timer wake source

	WakeAlways = WakeSensors | WakeExternal
)

// minDeepSleep is the minimum time remaining before a target deadline
// for deep sleep to be worthwhile. Below this threshold the overhead
// of alarm setup risks the alarm firing before Standby, and idle
// sleep is cheaper anyway.
const minDeepSleep = 2 * time.Second

// extPin associates an external interrupt pin with a group slot.
type extPin struct {
	pin  uint8
	slot int
}

// System composes optional hardware components into a unified
// sleep/wake platform. The processor is always required. RTC and
// Rails are optional — pass nil for graceful degradation:
//   - nil RTC: use time.Now(), no alarm-based wake
//   - nil Rails: no rail control
//
// System tracks N deadline slots (one per config group). Each slot
// has an interval and a rolling next-fire time. External interrupt
// pins can be associated with specific slots via RegisterExternalPin.
type System struct {
	mcu   MCU
	rtc   RTC
	rails Rails

	wakePins []uint8

	intervals []time.Duration
	deadlines []time.Time
	fired     []bool

	extPins []extPin
}

// NewSystem creates a System. mcu is required. rtc and rails may be
// nil. intervals has one entry per group slot — 0 means no timer for
// that slot. If an RTC is provided its wake pin is registered
// automatically.
func NewSystem(mcu MCU, rtc RTC, rails Rails, intervals []time.Duration) *System {
	n := len(intervals)
	s := &System{
		mcu:       mcu,
		rtc:       rtc,
		rails:     rails,
		intervals: make([]time.Duration, n),
		deadlines: make([]time.Time, n),
		fired:     make([]bool, n),
	}
	copy(s.intervals, intervals)
	if rtc != nil {
		s.wakePins = append(s.wakePins, rtc.WakePin())
	}
	return s
}

// RegisterExternalPin associates a GPIO interrupt pin with a group
// slot. When the system wakes from an external interrupt (no deadline
// fired), all slots with registered external pins are marked as fired.
// The pin is also added to the set of deep-sleep wake sources.
func (s *System) RegisterExternalPin(pin uint8, slot int) {
	s.extPins = append(s.extPins, extPin{pin: pin, slot: slot})

	// Add to wake pin set if not already present.
	if slices.Contains(s.wakePins, pin) {
		return
	}

	s.wakePins = append(s.wakePins, pin)
}

// AddWakePin registers an additional GPIO pin as a deep-sleep wake
// source that is not associated with any group slot. On external
// wake these pins contribute to the wake but don't fire any group.
func (s *System) AddWakePin(pin uint8) {
	s.wakePins = append(s.wakePins, pin)
}

// ReadTime returns the current time from the RTC, or time.Now() if
// no RTC is attached.
func (s *System) ReadTime() (time.Time, error) {
	if s.rtc != nil {
		return s.rtc.ReadTime()
	}
	return time.Now(), nil
}

// NextWake returns the duration until the nearest deadline fires.
// Returns 0 if no deadlines are active.
func (s *System) NextWake() time.Duration {
	now, err := s.ReadTime()
	if err != nil {
		now = time.Now()
	}

	var nearest time.Duration
	for i, d := range s.intervals {
		if d <= 0 {
			continue
		}
		var remaining time.Duration
		if !s.deadlines[i].IsZero() {
			remaining = max(s.deadlines[i].Sub(now), 0)
		} else {
			remaining = d
		}
		if nearest == 0 || remaining < nearest {
			nearest = remaining
		}
	}

	return nearest
}

// EnableSensorRails powers on sensor-specific rails (those tagged
// with WakeSensors). The manager calls this after determining that
// at least one fired group has sensors. No-op if rails is nil.
func (s *System) EnableSensorRails() {
	if s.rails != nil {
		s.rails.PowerOn(WakeSensors)
	}
}

// Sleep executes a single sleep/wake cycle. It tracks per-slot
// deadlines across calls, sleeps until the earliest fires, and
// returns which slots fired.
//
// The returned slice is owned by System and reused across calls.
// The caller must not retain a reference across Sleep invocations.
func (s *System) Sleep() ([]bool, error) {
	var sleepErrs []error

	// Reset fired state.
	for i := range s.fired {
		s.fired[i] = false
	}

	s.mcu.PetWatchdog()

	now, err := s.ReadTime()
	if err != nil {
		sleepErrs = append(sleepErrs, err)
		now = time.Now()
	}

	s.mcu.PetWatchdog()

	// Initialize any unset deadlines.
	for i, d := range s.intervals {
		if d > 0 && s.deadlines[i].IsZero() {
			s.deadlines[i] = now.Add(d)
		}
	}

	target, hasDeadlines := s.earliestDeadline()

	if now.Before(target) || !hasDeadlines {
		s.mcu.PetWatchdog()

		var remaining time.Duration
		if hasDeadlines {
			remaining = target.Sub(now)
		}

		// Deep sleep requires wake pins and either no deadlines to
		// miss, or an RTC with enough time remaining to justify the
		// overhead to enter standby mode (based on minDeepSleep).
		if len(s.wakePins) > 0 && (!hasDeadlines ||
			(s.rtc != nil && remaining > minDeepSleep)) {
			if err := s.deepSleep(target); err != nil {
				sleepErrs = append(sleepErrs, err)
				s.idleSleep(remaining)
			}
		} else {
			s.idleSleep(remaining)
		}

		s.mcu.PetWatchdog()

		// Restore core rails so the RTC and SD card are reachable.
		if s.rails != nil {
			s.rails.PowerOn(WakeAlways)
		}

		if s.rtc != nil {
			_ = s.rtc.ClearWake() // best-effort
		}

		s.mcu.PetWatchdog()

		now, err = s.ReadTime()
		if err != nil {
			sleepErrs = append(sleepErrs, err)
			now = time.Now()
		}

		s.mcu.PetWatchdog()
	}

	// Resolve which deadline slots fired.
	for i, d := range s.intervals {
		if d > 0 && !s.deadlines[i].IsZero() && !now.Before(s.deadlines[i]) {
			s.fired[i] = true
			for !now.Before(s.deadlines[i]) {
				s.deadlines[i] = s.deadlines[i].Add(d)
			}
		}
	}

	// Mark slots whose external interrupt pin fired. Checked
	// independently of deadlines since both can trigger during
	// the same sleep cycle.
	for _, ep := range s.extPins {
		if ep.slot < len(s.fired) && s.mcu.PinFired(ep.pin) {
			s.fired[ep.slot] = true
		}
	}

	return s.fired, errors.Join(sleepErrs...)
}

// deepSleep sets the RTC alarm (if present), arms all wake pins,
// cuts power, and enters MCU standby.
func (s *System) deepSleep(target time.Time) error {
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
// between each, until d elapses. The tick duration is such that
func (s *System) idleSleep(d time.Duration) {
	const tick = 4 * time.Second

	if d <= 0 {
		for {
			s.mcu.PetWatchdog()
			wait.For(tick)
		}
	}

	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		s.mcu.PetWatchdog()
		remaining := min(time.Until(deadline), tick)
		wait.For(remaining)
	}
}

// earliestDeadline returns the earliest non-zero deadlines entry and
// whether any deadlines exist at all.
func (s *System) earliestDeadline() (time.Time, bool) {
	var best time.Time
	for i, d := range s.intervals {
		if d <= 0 {
			continue
		}
		t := s.deadlines[i]
		if t.IsZero() {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best, !best.IsZero()
}
