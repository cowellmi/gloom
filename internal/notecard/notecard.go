package notecard

import (
	"errors"
	"strings"

	tinynote "github.com/blues/note-tinygo"
)

type Device struct {
	UID string
	ctx *tinynote.Context
}

func New(tx tinynote.I2CTxFn) (*Device, error) {
	var d Device
	var err error
	d.ctx, err = tinynote.OpenI2C(tinynote.DefaultI2CAddress, tx)
	if err != nil {
		return nil, err
	}

	req := tinynote.NewRequest("card.version")
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return nil, errors.New(tinynote.ErrorString(err, rsp))
	}

	if deviceUID, ok := rsp["device"].(string); ok {
		d.UID = deviceUID
	}

	req = tinynote.NewRequest("hub.sync")
	if err = d.ctx.Request(req); tinynote.IsError(err, rsp) {
		return nil, errors.New(tinynote.ErrorString(err, rsp))
	}

	return &d, nil
}

func (d *Device) Request(req map[string]any) error {
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return errors.New(tinynote.ErrorString(err, rsp))
	}
	return nil
}

func (d *Device) RequestResponse(req map[string]any) (map[string]any, error) {
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return nil, errors.New(tinynote.ErrorString(err, rsp))
	}
	return rsp, nil
}

func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "note-noexist")
}
