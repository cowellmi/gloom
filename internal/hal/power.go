package hal

import "time"

// PowerManager abstracts board-level power rail control. Boards with
// MOSFET-switched rails (e.g. Hypnos) implement this to cut power to
// sensors during sleep. Boards without rail control simply omit the
// PowerManager from the System.
type PowerManager interface {
	// PowerOn enables power rails. Called after waking from sleep.
	PowerOn()

	// PowerOff disables power rails. Called before entering sleep.
	PowerOff()

	// Delay returns how long to wait after PowerOn for voltages to
	// stabilise before talking to peripherals.
	Delay() time.Duration
}
