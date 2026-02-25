package fallback

import (
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

type Rails struct{}

func (Rails) Power(hal.RailState) {}

type LED struct{}

func (LED) On()          {}
func (LED) Off()         {}
func (LED) Blink()       {}
func (LED) Pin() hal.Pin { return hal.NoPin }

type RTC struct{}

func (RTC) Identifier() string           { return "fallback" }
func (RTC) HasAlarm() bool               { return false }
func (RTC) ReadTime() (time.Time, error) { return time.Now(), nil }
func (RTC) SetAlarm(time.Time) error     { return hal.ErrNoAlarm }
func (RTC) ClearAlarm() error            { return hal.ErrNoAlarm }
