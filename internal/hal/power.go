package hal

import "time"

// PowerManager abstracts board-level power rail control. Each rail is
// tagged with a WakeReason bitmask that determines when it activates:
//
//   - WakeAlways rails (core): powered on every wake so the RTC and
//     SD card are accessible. Brought up immediately after wake.
//   - WakeSample rails: only powered on when sampling sensors.
//   - WakeHeartbeat rails: only powered on for heartbeat transmit.
//
// PowerOn is called twice per wake: once with WakeAlways to bring up
// core infrastructure, and again with the actual wake reason to bring
// up reason-specific rails. A rail activates when its tag ANDs with
// the reason.
//
// Boards without rail control simply omit the PowerManager from the
// System (pass nil to NewSystem).
type PowerManager interface {
	// PowerOn enables all rails whose WakeReason tag overlaps with
	// reason. Rails already on are left unchanged.
	PowerOn(reason WakeReason)

	// PowerOff disables all power rails. Called before entering sleep.
	PowerOff()

	// Delay returns how long to wait after PowerOn for voltages to
	// stabilise before talking to peripherals.
	Delay() time.Duration
}
