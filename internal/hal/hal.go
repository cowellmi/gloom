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

// System composes optional hardware components into a unified
// sleep/wake platform. The processor is always required (if the MCU
// isn't running, the firmware isn't running). RTC and Rails are
// optional — pass nil for graceful degradation:
//   - nil RTC: use time.Now(), no alarm-based wake, no deep sleep
//   - nil Rails: no rail control
type System struct {
	mcu   MCU
	rtc   RTC
	rails Rails

	nextSample    time.Time
	nextHeartbeat time.Time
}

// NewSystem creates a System. mcu is required. rtc and rails may be nil.
func NewSystem(mcu MCU, rtc RTC, rails Rails) *System {
	return &System{mcu: mcu, rtc: rtc, rails: rails}
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

// Sleep executes a single sleep/wake cycle. It tracks sample and
// heartbeat deadlines across calls, sleeps until the earliest
// deadline, and returns the reason the system woke.
//
// The sleep strategy depends on available components:
//   - With RTC: set RTC alarm, arm wake pin, MCU standby (deep sleep)
//   - Without RTC: busy-wait using internal clock (no deep sleep)
//
// Power rail behaviour when Rails is attached:
//   - All rails are cut before sleep.
//   - On wake, core rails (WakeAlways) are restored first so the RTC
//     and SD card are accessible.
//   - After the wake reason is determined, reason-specific rails are
//     enabled (e.g. WakeSample rails for sensor power). This keeps
//     sensor rails off during heartbeat-only wakes, saving power.
func (s *System) Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error) {
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
	if sampleInterval > 0 && s.nextSample.IsZero() {
		adj := sampleInterval
		// Subtract sensor power-on delay so sensors are ready by
		// the nominal sample time.
		if s.rails != nil {
			adj -= s.rails.Delay()
		}
		if adj < 0 {
			adj = 0
		}
		s.nextSample = now.Add(adj)
	}
	if heartbeatInterval > 0 && s.nextHeartbeat.IsZero() {
		s.nextHeartbeat = now.Add(heartbeatInterval)
	}

	target := earliest(s.nextSample, s.nextHeartbeat)

	// --- Sleep until target ---
	if now.Before(target) || target.IsZero() {
		s.mcu.PetWatchdog()

		// Attempt deep sleep if we have an RTC.
		if s.rtc != nil {
			err := s.deepSleep(target)
			if err != nil {
				sleepErrs = append(sleepErrs, err)
				// Deep sleep failed — fall back to idle wait.
				s.idleSleep(target)
			}
		} else {
			// No RTC — idle wait only.
			s.idleSleep(target)
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
	if sampleInterval > 0 && !s.nextSample.IsZero() && !now.Before(s.nextSample) {
		reason = WakeSample
		s.nextSample = time.Time{}
	} else if heartbeatInterval > 0 && !s.nextHeartbeat.IsZero() && !now.Before(s.nextHeartbeat) {
		reason = WakeHeartbeat
		s.nextHeartbeat = time.Time{}
	} else {
		reason = WakeExternal
	}

	// --- Power on reason-specific rails ---
	if s.rails != nil && reason != WakeExternal {
		s.rails.PowerOn(reason)
		wait.For(s.rails.Delay())
		s.mcu.PetWatchdog()
	}

	return reason, errors.Join(sleepErrs...)
}

// deepSleep sets the RTC alarm, arms the wake pin, cuts power, and
// enters MCU standby. On wake it clears the alarm and reads the time.
func (s *System) deepSleep(target time.Time) error {
	// Set RTC alarm if a timed deadline is active.
	if !target.IsZero() {
		if err := s.rtc.ClearWake(); err != nil {
			return err
		}
		if err := s.rtc.SetWake(target); err != nil {
			return err
		}
	}

	s.mcu.PetWatchdog()

	// Cut power rails before arming the wake interrupt. The
	// DS3231 INT pin can glitch during the 3.3V→battery
	// transition. If the EIC is already armed, that glitch
	// fires the ISR (which disarms the wake source), and the
	// real alarm later has nothing to wake the CPU.
	if s.rails != nil {
		s.rails.PowerOff()
	}

	// Arm the MCU wake source on the RTC interrupt pin.
	if err := s.mcu.ArmWake(s.rtc.WakePin()); err != nil {
		s.mcu.DisarmWake(s.rtc.WakePin())
		return err
	}

	// Disable the watchdog before standby. The WDT GCLK is
	// configured without RUNSTDBY, but in practice the WDT
	// still fires during standby (possibly due to stale GCLK0
	// routing from TinyGo's runtime init). Disabling it
	// explicitly prevents resets during intentional sleep.
	s.mcu.DisableWatchdog()

	// Enter deep sleep — execution halts until wake interrupt.
	s.mcu.Standby()

	s.mcu.EnableWatchdog()
	s.mcu.PetWatchdog()

	return nil
}

// idleSleep busy-waits in short intervals, petting the watchdog
// between each, until the target time is reached. Used when deep
// sleep is unavailable or fails.
func (s *System) idleSleep(target time.Time) {
	const tick = 4 * time.Second // well within ~8s watchdog timeout

	if target.IsZero() {
		wait.For(tick)
		return
	}

	for time.Now().Before(target) {
		s.mcu.PetWatchdog()
		remaining := time.Until(target)
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
