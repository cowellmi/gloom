//go:build feather_m0 && no_hypnos

package main

import "github.com/cowellmi/gloom/internal/fallback"

// No wing attached; use fallbacks.
func initWing() Wing {
	return Wing{
		Rails: fallback.Rails{},
	}
}
