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
	"time"

	"github.com/cowellmi/gloom/internal/wait"
)

// WakeReason is a bitmask identifying why the system woke. Power
// rails use WakeReason to decide which rails to enable: a rail tagged
// with WakeSample only powers on when the wake reason includes
// WakeSample. WakeAlways matches every reason and is used for core
// infrastructure rails (RTC, SD card).
type WakeReason uint8

const (
	WakeSample    WakeReason = 1 << iota // 0b001
	WakeHeartbeat                        // 0b010
	WakeExternal                         // 0b100

	WakeAlways = WakeSample | WakeHeartbeat | WakeExternal // 0b111
)

// minDeepSleep is the minimum time remaining before a target deadline
// for deep sleep to be worthwhile. Below this threshold the overhead
// of alarm setup risks the alarm firing before Standby, and idle
// sleep is cheaper anyway.
const minDeepSleep = 2 * time.Second

// System composes optional hardware components into a unified
// sleep/wake platform. The processor is always required (if the MCU
// isn't running, the firmware isn't running). RTC and Rails are
// optional — pass nil for graceful degradation:
//   - nil RTC: use time.Now(), no alarm-based wake
//   - nil Rails: no rail control
//
// Deep sleep requires at least one wake pin. When an RTC is provided
// its interrupt pin is added automatically. Additional pins (buttons,
// sensor threshold lines) can be registered via AddWakePin.
type System struct {
	mcu   MCU
	rtc   RTC
	rails Rails

	wakePins []uint8

	sampleInterval    time.Duration
	heartbeatInterval time.Duration
	nextSample        time.Time
	nextHeartbeat     time.Time
}

// NewSystem creates a System. mcu is required. rtc and rails may be
// nil. If an RTC is provided its wake pin is registered automatically.
// sampleInterval and heartbeatInterval configure the sleep/wake
// cadence; pass 0 to disable either.
func NewSystem(mcu MCU, rtc RTC, rails Rails, sampleInterval, heartbeatInterval time.Duration) *System {
	s := &System{
		mcu:               mcu,
		rtc:               rtc,
		rails:             rails,
		sampleInterval:    sampleInterval,
		heartbeatInterval: heartbeatInterval,
	}
	if rtc != nil {
		s.wakePins = append(s.wakePins, rtc.WakePin())
	}
	return s
}

// AddWakePin registers an additional GPIO pin as a deep-sleep wake
// source (e.g. a tipping-bucket interrupt or pushbutton). The pin
// will be armed alongside the RTC pin before each standby entry.
func (s *System) AddWakePin(pin uint8) {
	s.wakePins = append(s.wakePins, pin)
}

func (s *System) Identifier() string {
	return s.mcu.Identifier()
}

// ReadTime returns the current time from the RTC, or time.Now() if
// no RTC is attached.
func (s *System) ReadTime() (time.Time, error) {
	if s.rtc != nil {
		return s.rtc.ReadTime()
	}
	return time.Now(), nil
}

// NextWake returns the time remaining until each scheduled wake
// deadline. For a deadline that hasn't been set yet (first call, or
// just after the deadline fired and was cleared), the full interval
// is returned since Sleep will set it to now+interval. A disabled
// interval (<=0) always returns 0.
func (s *System) NextWake() (sample, heartbeat time.Duration) {
	now, err := s.ReadTime()
	if err != nil {
		now = time.Now()
	}

	if s.sampleInterval > 0 {
		if !s.nextSample.IsZero() {
			sample = max(s.nextSample.Sub(now), 0)
		} else {
			sample = s.sampleInterval
		}
	}

	if s.heartbeatInterval > 0 {
		if !s.nextHeartbeat.IsZero() {
			heartbeat = max(s.nextHeartbeat.Sub(now), 0)
		} else {
			heartbeat = s.heartbeatInterval
		}
	}

	return sample, heartbeat
}

// Sleep executes a single sleep/wake cycle. It tracks sample and
// heartbeat deadlines across calls, sleeps until the earliest
// deadline, and returns the reason the system woke.
//
// The sleep strategy depends on available wake sources:
//   - With wake pins: arm all pins, set RTC alarm (if RTC present),
//     MCU standby (deep sleep)
//   - Without wake pins: busy-wait using internal clock (no deep sleep)
//
// Power rail behaviour when Rails is attached:
//   - All rails are cut before sleep.
//   - On wake, core rails (WakeAlways) are restored first so the RTC
//     and SD card are accessible.
//   - After the wake reason is determined, reason-specific rails are
//     enabled (e.g. WakeSample rails for sensor power). This keeps
//     sensor rails off during heartbeat-only wakes, saving power.
func (s *System) Sleep() (WakeReason, error) {
	var sleepErrs []error

	s.mcu.PetWatchdog()

	// --- Read current time ---
	now, err := s.ReadTime()
	if err != nil {
		sleepErrs = append(sleepErrs, err)
		now = time.Now()
	}

	s.mcu.PetWatchdog()

	// --- Update deadlines ---
	if s.sampleInterval > 0 && s.nextSample.IsZero() {
		adj := s.sampleInterval
		// Subtract sensor power-on delay so sensors are ready by
		// the nominal sample time.
		if s.rails != nil {
			adj -= s.rails.Delay()
		}
		adj = max(adj, 0)
		s.nextSample = now.Add(adj)
	}
	if s.heartbeatInterval > 0 && s.nextHeartbeat.IsZero() {
		s.nextHeartbeat = now.Add(s.heartbeatInterval)
	}

	target := earliest(s.nextSample, s.nextHeartbeat)

	// If target is zero, that means neither the sample interval nor
	// the heartbeat interval is set. This is a valid state: it means
	// that the user is expecting some external interrupt (like a sensor)
	// to wake the device.
	noDeadlines := target.IsZero()

	// --- Sleep until target ---
	if now.Before(target) || noDeadlines {
		s.mcu.PetWatchdog()

		// Deep sleep requires at least one wake pin. When the RTC
		// is providing a timed alarm, we also need enough margin so
		// the alarm doesn't fire during setup (before Standby),
		// which would leave the INT pin LOW with no falling edge
		// to wake the CPU.
		remaining := target.Sub(now)
		if noDeadlines {
			remaining = 0
		}

		canDeepSleep := len(s.wakePins) > 0
		if canDeepSleep && s.rtc != nil && !noDeadlines {
			canDeepSleep = remaining > minDeepSleep
		}

		if canDeepSleep {
			err := s.deepSleep(target)
			if err != nil {
				sleepErrs = append(sleepErrs, err)
				// Deep sleep failed — fall back to idle wait.
				s.idleSleep(remaining)
			}
		} else {
			s.idleSleep(remaining)
		}

		s.mcu.PetWatchdog()

		// Restore core rails (WakeAlways) so the RTC and SD card
		// are reachable. Reason-specific rails stay off until we
		// know which wake reason fired.
		if s.rails != nil {
			s.rails.PowerOn(WakeAlways)
		}

		// Clear the RTC alarm so the interrupt pin releases.
		if s.rtc != nil {
			_ = s.rtc.ClearWake() // best-effort
		}

		s.mcu.PetWatchdog()

		// Re-read time after wake.
		now, err = s.ReadTime()
		if err != nil {
			sleepErrs = append(sleepErrs, err)
			now = time.Now()
		}

		s.mcu.PetWatchdog()
	}

	// --- Resolve wake reason ---
	var reason WakeReason
	if s.sampleInterval > 0 && !s.nextSample.IsZero() && !now.Before(s.nextSample) {
		reason = WakeSample
		s.nextSample = time.Time{}
	} else if s.heartbeatInterval > 0 && !s.nextHeartbeat.IsZero() && !now.Before(s.nextHeartbeat) {
		reason = WakeHeartbeat
		s.nextHeartbeat = time.Time{}
	} else {
		reason = WakeExternal
	}

	// --- Power on reason-specific rails ---
	// Only wait for rail stabilisation on sample wakes where sensors
	// need power. Heartbeat wakes use only the core rails already
	// restored above and don't need the delay.
	if s.rails != nil && reason == WakeSample {
		s.rails.PowerOn(reason)
		wait.For(s.rails.Delay())
		s.mcu.PetWatchdog()
	}

	return reason, errors.Join(sleepErrs...)
}

// deepSleep sets the RTC alarm (if present), arms all wake pins,
// cuts power, and enters MCU standby. On wake it disarms the pins
// and restores the watchdog.
func (s *System) deepSleep(target time.Time) error {
	// Set RTC alarm if a timed deadline is active.
	if s.rtc != nil && !target.IsZero() {
		if err := s.rtc.ClearWake(); err != nil {
			return err
		}
		if err := s.rtc.SetWake(target); err != nil {
			return err
		}
	}

	s.mcu.PetWatchdog()

	// Cut power rails before arming wake interrupts so the
	// DS3231 INT pin glitch during the 3.3V→battery transition
	// is less likely to fire a spurious EIC interrupt.
	if s.rails != nil {
		s.rails.PowerOff()
	}

	// Arm all registered wake pins. If any fails, disarm the
	// ones that succeeded and bail out.
	for i, pin := range s.wakePins {
		if err := s.mcu.ArmWake(pin); err != nil {
			for j := 0; j <= i; j++ {
				s.mcu.DisarmWake(s.wakePins[j])
			}
			return err
		}
	}

	// Disable the watchdog before standby. The WDT GCLK is
	// configured without RUNSTDBY, but in practice the WDT
	// still fires during standby (possibly due to stale GCLK0
	// routing from TinyGo's runtime init). Disabling it
	// explicitly prevents resets during intentional sleep.
	s.mcu.DisableWatchdog()

	// Enter deep sleep — execution halts until wake interrupt.
	s.mcu.Standby()

	// Disarm all wake pins now that we're awake. The ISRs are
	// no-ops, so interrupt registrations and EIC WAKEUP bits are
	// still set from ArmWake — clean them up here.
	for _, pin := range s.wakePins {
		s.mcu.DisarmWake(pin)
	}

	s.mcu.EnableWatchdog()
	s.mcu.PetWatchdog()

	return nil
}

// idleSleep busy-waits in short intervals, petting the watchdog
// between each, until d elapses. Used when deep sleep is unavailable
// or the remaining time is too short to justify it. Takes a duration
// rather than an absolute time so the caller can translate from
// RTC-sourced deadlines without clock-domain mismatch (the system
// monotonic clock may differ from the RTC).
func (s *System) idleSleep(d time.Duration) {
	const tick = 4 * time.Second // well within ~8s watchdog timeout

	if d <= 0 {
		wait.For(tick)
		return
	}

	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		s.mcu.PetWatchdog()
		remaining := time.Until(deadline)
		if remaining > tick {
			remaining = tick
		}
		wait.For(remaining)
	}
}

// earliest returns the earlier of two times. If either is zero
// (unscheduled), the other is returned. If both are zero, zero is
// returned.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.Before(b) {
		return a
	}
	return b
}
