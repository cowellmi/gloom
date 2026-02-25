//go:build feather_m0 && no_hypnos

package main

// initWakePin returns NoPin when Hypnos is not attached.
func initDevices() Devices {
	return Devices{}
}
