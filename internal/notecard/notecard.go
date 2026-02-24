package notecard

import (
	"errors"

	tinynote "github.com/blues/note-tinygo"
	"github.com/cowellmi/gloom/internal/config"
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
	d.UID, _ = rsp["device"].(string)

	return &d, nil
}

func (d *Device) Configure(cfg *config.Config) error {
	req := tinynote.NewRequest("env.template")
	req["body"] = cfg.MarshalMap()
	if err := d.ctx.Request(req); err != nil {
		return err
	}

	req = tinynote.NewRequest("env.get")
	rsp, err := d.ctx.RequestResponse(req)
	if tinynote.IsError(err, rsp) {
		return errors.New(tinynote.ErrorString(err, rsp))
	}

	body, ok := rsp["body"].(map[string]any)
	if !ok {
		return errors.New("invalid response")
	}

	env := make(map[string]any)
	for k, v := range body {
		if len(k) > 0 && k[0] == '_' {
			continue // reserved
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		env[k] = v
	}

	if len(env) > 0 {
		return config.ParseMap(cfg, env)
	}

	for k, v := range cfg.MarshalMap() {
		if vStr, ok := v.(string); ok {
			req := tinynote.NewRequest("env.default")
			req["name"] = k
			req["text"] = vStr
			if err := d.ctx.Request(req); err != nil {
				return err
			}
		}
	}

	return nil
}
