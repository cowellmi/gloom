package hal

// Rails abstracts board-level power rail control. Each rail is
// tagged with a WakeReason bitmask that determines when it activates:
//
//   - WakeAlways rails (core): powered on every wake so the RTC and
//     SD card are accessible. Brought up immediately after wake.
//   - WakeSensors rails: only powered on when at least one fired
//     group has sensors. The manager calls EnableSensorRails() after
//     resolving fired groups.
//
// PowerOn is called with WakeAlways after every wake to bring up core
// infrastructure. A second call with WakeSensors enables sensor rails
// when needed. A rail activates when its tag ANDs with the reason.
//
// Boards without rail control simply omit Rails from the System
// (pass nil to NewSystem).
type Rails interface {
	// PowerOn enables all rails whose WakeReason tag overlaps with
	// reason. Rails already on are left unchanged.
	PowerOn(reason WakeReason)

	// PowerOff disables all power rails. Called before entering sleep.
	PowerOff()
}
