package power

import "machine"

type Controller struct {
	pin machine.Pin
}

func New(pin machine.Pin) *Controller {
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	return &Controller{
		pin: pin,
	}
}

func (c *Controller) On() {
	c.pin.High()
}

func (c *Controller) Off() {
	c.pin.Low()
}
