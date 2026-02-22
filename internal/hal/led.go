package hal

type LED interface {
	Configure(pin uint8)
	On()
	Off()
}
