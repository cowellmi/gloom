package hal

// RailState describes the desired state of power rails.
type RailState uint8

const (
	// RailsOff means all rails disabled. Used before deep sleep.
	RailsOff RailState = iota
	// RailsCore enables rails with threshold <= RailsCore (core
	// infrastructure like RTC and SD card). Used after wake and
	// during idle sleep.
	RailsCore
	// RailsFull enables all rails. Used when sensors are active.
	RailsFull
)

// Rails abstracts board-level power rail control. Each rail has a
// threshold: it is enabled when Power(state) is called with
// state >= threshold, and disabled otherwise.
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
