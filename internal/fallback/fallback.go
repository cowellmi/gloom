package fallback

import (
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

type Rails struct{}

func (Rails) Identifier() string  { return "none" }
func (Rails) Power(hal.RailState) {}

type LED struct{}

func (LED) On()          {}
func (LED) Off()         {}
func (LED) Blink()       {}
func (LED) Pin() hal.Pin { return hal.NoPin }

type Clock struct{}

func (Clock) Identifier() string           { return "none" }
func (Clock) ReadTime() (time.Time, error) { return time.Now(), nil }
