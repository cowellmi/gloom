// Package hal defines the hardware abstraction layer for Gloom.
//
// The central type is System, a composable struct that assembles
// optional hardware components (RTC, power manager) alongside a
// required MCU processor into a unified sleep/wake interface. All pin
// references use uint8 so this package remains testable with the
// standard Go toolchain.
package hal

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/wait"
)

// Processor abstracts the MCU operations needed by the System for
// deep sleep. This is a hal-local interface satisfied by mcu.MCU
// implementations. It uses uint8 for pin numbers to avoid importing
// the machine package.
type Processor interface {
	Identifier() string
	PetWatchdog()

	// ArmWake configures pin as a wake source: sets it as input
	// pullup, registers a falling-edge interrupt, prepares standby
	// clocks, and enables the EIC wakeup bit. The interrupt handler
	// is internal and disarms the wake source on fire.
	ArmWake(pin uint8) error

	// DisarmWake tears down the wake source for pin.
	DisarmWake(pin uint8)

	// Standby puts the processor into its deepest sleep mode.
	Standby()
}

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
// isn't running, the firmware isn't running). RTC and PowerManager
// are optional — pass nil for graceful degradation:
//   - nil RTC: use time.Now(), no alarm-based wake, no deep sleep
//   - nil PowerManager: no rail control
type System struct {
	proc  Processor
	rtc   RTC
	power PowerManager

	nextSample    time.Time
	nextHeartbeat time.Time
}

// NewSystem creates a System. proc is required. rtc and pm may be nil.
func NewSystem(proc Processor, rtc RTC, pm PowerManager) *System {
	return &System{proc: proc, rtc: rtc, power: pm}
}

func (s *System) Identifier() string {
	return s.proc.Identifier()
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
// Power rail behaviour when a PowerManager is attached:
//   - All rails are cut before sleep.
//   - On wake, core rails (WakeAlways) are restored first so the RTC
//     and SD card are accessible.
//   - After the wake reason is determined, reason-specific rails are
//     enabled (e.g. WakeSample rails for sensor power). This keeps
//     sensor rails off during heartbeat-only wakes, saving power.
func (s *System) Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error) {
	var sleepErrs []error

	s.proc.PetWatchdog()

	// --- Read current time ---
	now, err := s.ReadTime()
	if err != nil {
		sleepErrs = append(sleepErrs, err)
		now = time.Now()
	}

	s.proc.PetWatchdog()

	// --- Update deadlines ---
	if sampleInterval > 0 && s.nextSample.IsZero() {
		adj := sampleInterval
		// Subtract sensor power-on delay so sensors are ready by
		// the nominal sample time.
		if s.power != nil {
			adj -= s.power.Delay()
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
		s.proc.PetWatchdog()

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

		s.proc.PetWatchdog()

		// Restore core rails (WakeAlways) so the RTC and SD card
		// are reachable. Reason-specific rails stay off until we
		// know which wake reason fired.
		if s.power != nil {
			s.power.PowerOn(WakeAlways)
		}

		s.proc.PetWatchdog()

		// Re-read time after wake.
		now, err = s.ReadTime()
		if err != nil {
			sleepErrs = append(sleepErrs, err)
			now = time.Now()
		}

		s.proc.PetWatchdog()
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
	if s.power != nil && reason != WakeExternal {
		s.power.PowerOn(reason)
		wait.For(s.power.Delay())
		s.proc.PetWatchdog()
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

	// Arm the MCU wake source on the RTC interrupt pin.
	if err := s.proc.ArmWake(s.rtc.WakePin()); err != nil {
		s.proc.DisarmWake(s.rtc.WakePin())
		return err
	}

	s.proc.PetWatchdog()

	// Cut power rails.
	if s.power != nil {
		s.power.PowerOff()
	}

	// Enter deep sleep — execution halts until wake interrupt.
	s.proc.Standby()

	s.proc.PetWatchdog()

	// Clear the RTC alarm so the interrupt pin releases.
	_ = s.rtc.ClearWake() // best-effort

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
		s.proc.PetWatchdog()
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
