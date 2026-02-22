//go:build feather_m0 && no_hypnos

package main

// initRails is a no-op when the Hypnos FeatherWing is not attached.
func initRails(_ *Board) {}
