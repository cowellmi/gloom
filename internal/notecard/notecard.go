package notecard

import (
	"errors"
	"strings"

	tinynote "github.com/blues/note-tinygo"
)

// Requester is the minimal transport interface for Blues API requests.
type Requester interface {
	Request(req map[string]any) error
	RequestResponse(req map[string]any) (map[string]any, error)
}

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
	d.UID, _ = rsp["device"].(string)

	return &d, nil
}

// Request sends a Notecard request and discards the response body.
func (d *Device) Request(req map[string]any) error {
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return errors.New(tinynote.ErrorString(err, rsp))
	}
	return nil
}

// RequestResponse sends a Notecard request and returns the response map.
func (d *Device) RequestResponse(req map[string]any) (map[string]any, error) {
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return nil, errors.New(tinynote.ErrorString(err, rsp))
	}
	return rsp, nil
}

// IsNotFound reports whether err is a Blues "note does not exist" response,
// used to distinguish an empty Notefile from a real transport failure.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "note-noexist")
}
