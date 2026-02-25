package fallback

import "github.com/cowellmi/gloom/internal/hal"

type Rails struct{}

func (Rails) Power(hal.RailState) {}

type LED struct{}

func (LED) On()          {}
func (LED) Off()         {}
func (LED) Blink()       {}
func (LED) Pin() hal.Pin { return hal.NoPin }
