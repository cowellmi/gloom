package hal

// Rails abstracts board-level power rail control. Each rail is either
// "always" (powered every wake for core infrastructure like RTC and
// SD card) or on-demand (powered only when fired groups need sensors).
//
// PowerOn(false) is called after every wake to bring up always-rails.
// PowerOn(true) is called by the manager when at least one fired group
// has sensors.
//
// Boards without rail control simply omit Rails from the System
// (pass nil to NewSystem).
type Rails interface {
	// PowerOn enables rails. When sensors is false, only always-rails
	// are enabled. When true, all rails are enabled.
	PowerOn(sensors bool)

	// PowerOff disables all power rails. Called before entering sleep.
	PowerOff()
}
