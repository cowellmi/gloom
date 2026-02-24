package hal

type LED interface {
	On()
	Off()

	Pin() Pin

	// Flash on/off, on/off
	Blink()
}
