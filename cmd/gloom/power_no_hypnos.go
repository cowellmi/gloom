//go:build feather_m0 && no_hypnos

package main

// initRails returns no rails when the Hypnos FeatherWing is not attached.
func initRails() ([]RailConfig, time.Duration) { return nil, 0 }
