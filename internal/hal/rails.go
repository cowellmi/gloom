package hal

// RailState describes the desired state of power rails.
type RailState uint8

const (
	// RailsOff means all rails disabled. Used before deep sleep.
	RailsOff RailState = iota
	// RailsCore means always-rails enabled, on-demand rails disabled.
	// Used after wake to enable RTC/SD, and during idle sleep.
	RailsCore
	// RailsFull means all rails enabled. Used when sensors are active.
	RailsFull
)

// Rails abstracts board-level power rail control. Each rail is either
// "always" (powered every wake for core infrastructure like RTC and
// SD card) or on-demand (powered only when fired groups need sensors).
//
// The implementation handles per-rail stabilization delays internally
// when rails are being enabled.
//
// Boards without rail control simply omit Rails from the System
// (pass nil to NewSystem).
type Rails interface {
	// Power sets the power rail state. Waits for the stabilization
	// delay of any newly-enabled rails before returning.
	Power(state RailState)
}
