//go:build feather_m0 && no_hypnos

package main

import "github.com/cowellmi/gloom/internal/config"

// initRails is a no-op when the Hypnos FeatherWing is not attached.
func initRails(_ *config.Config) {}
