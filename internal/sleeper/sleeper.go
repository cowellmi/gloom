package sleeper

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/fallback"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"
)

const minDeepSleep = 2 * time.Second

type Device struct {
	mcu      hal.MCU
	rtc      hal.Clock
	rails    hal.Rails
	sda      hal.Pin
	scl      hal.Pin
	wakePins []hal.Pin
}

func New(mcu hal.MCU, rtc hal.Clock, rails hal.Rails, sda, scl hal.Pin, interruptPins []hal.Pin) *Device {
	return &Device{
		mcu:      mcu,
		rtc:      rtc,
		rails:    rails,
		sda:      sda,
		scl:      scl,
		wakePins: interruptPins,
	}
}

// PinFired reports whether the given pin fired during the most recent
// deep-sleep (Standby) cycle.
func (s *Device) PinFired(pin hal.Pin) bool {
	return s.mcu.PinFired(pin)
}

// Sleep enters deep or idle sleep until target, then returns the
// actual wake time from the RTC (or time.Now() on read failure).
// Any RTC read errors encountered during the cycle are joined and
// returned so the caller can log them as warnings.
//
// If target is already in the past, Sleep returns the current time
// immediately without entering hardware sleep.
func (s *Device) Sleep(target time.Time) (time.Time, error) {
	var errs []error
	now, err := s.readTime()
	errs = append(errs, err)

	remaining := target.Sub(now)
	if remaining > 0 {
		s.mcu.PetWatchdog()

		if len(s.wakePins) > 0 && remaining > minDeepSleep {
			if err := s.deepSleep(target); err != nil {
				s.rails.Power(hal.RailsCore)
				errs = append(errs, s.idleSleep(target))
			}
		} else {
			errs = append(errs, s.idleSleep(target))
		}

		s.mcu.PetWatchdog()

		s.rails.Power(hal.RailsCore)

		s.mcu.ConfigureI2C(s.sda, s.scl)

		if ac, ok := s.rtc.(hal.AlarmClock); ok {
			errs = append(errs, ac.ClearAlarm())
		}

		s.mcu.PetWatchdog()
	}

	wakeTime, err := s.readTime()
	errs = append(errs, err)
	return wakeTime, errors.Join(errs...)
}

func (s *Device) readTime() (time.Time, error) {
	now, err := s.rtc.ReadTime()
	if err != nil {
		s.rtc = fallback.Clock{}
		now, _ = s.rtc.ReadTime() // Fallback clock can't fail
		err = errors.Join(err, errors.New("rtc failed; using fallback clock"))
	}
	return now, err
}

// deepSleep sets the RTC alarm (if present), arms all wake pins,
// cuts power, and enters MCU standby.
func (s *Device) deepSleep(target time.Time) error {
	if ac, ok := s.rtc.(hal.AlarmClock); ok {
		if err := ac.ClearAlarm(); err != nil {
			return err
		}
		if err := ac.SetAlarm(target); err != nil {
			return err
		}
	}

	s.mcu.PetWatchdog()

	s.rails.Power(hal.RailsOff)

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
func (s *Device) idleSleep(target time.Time) error {
	const tick = 4 * time.Second

	var errs []error
	now, err := s.readTime()
	errs = append(errs, err)

	for now.Before(target) {
		s.mcu.PetWatchdog()
		remaining := min(target.Sub(now), tick)
		wait.For(remaining)
		now, err = s.readTime()
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
