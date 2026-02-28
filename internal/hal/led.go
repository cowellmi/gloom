package hal

type LED interface {
	On()
	Off()

	// Flash on/off, on/off
	Blink()
}
