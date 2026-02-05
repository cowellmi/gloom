package rtc

import (
	"errors"
	"strconv"
	"time"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	DSAlarm1 AlarmID = 1
	DSAlarm2 AlarmID = 2
)

type DS3231 struct {
	device ds3231.Device
}

func NewDS3231(bus drivers.I2C) (*DS3231, error) {
	clk := &DS3231{device: ds3231.New(bus)}

	if !clk.device.Configure() {
		return nil, errors.New("ds3231: internal driver configuration failed")
	}

	err := clk.device.SetRunning(true)
	if err != nil {
		return nil, err
	}

	err = clk.device.SetSqwPinMode(ds3231.SqwPinMode(ds3231.ModeAlarmBoth))
	if err != nil {
		return nil, err
	}

	return clk, nil
}

func (clk *DS3231) ClearAlarm(id AlarmID) error {
	switch id {
	case DSAlarm1:
		return clk.device.ClearAlarm1()

	case DSAlarm2:
		return clk.device.ClearAlarm2()

	default:
		return invalidAlarm(id)
	}
}

func (clk *DS3231) IsAlarmFired(id AlarmID) bool {
	switch id {
	case DSAlarm1:
		return clk.device.IsAlarm1Fired()

	case DSAlarm2:
		return clk.device.IsAlarm2Fired()

	default:
		return false
	}
}

func (clk *DS3231) ReadTime() (time.Time, error) {
	return clk.device.ReadTime()
}

func (clk *DS3231) SetAlarm(id AlarmID, duration time.Duration) error {
	now, err := clk.device.ReadTime()
	if err != nil {
		return err
	}
	wake := now.Add(duration)

	switch id {
	case DSAlarm1:
		err = clk.device.SetAlarm1(wake, ds3231.A1_SECOND)
		if err != nil {
			return err
		}

		return clk.device.SetEnabledAlarm1(true)

	case DSAlarm2:
		err = clk.device.SetAlarm2(wake, ds3231.A2_MINUTE)
		if err != nil {
			return err
		}

		return clk.device.SetEnabledAlarm2(true)

	default:
		return invalidAlarm(id)
	}
}

func invalidAlarm(id AlarmID) error {
	return errors.New("ds3231: invalid alarm id: " + strconv.Itoa(int(id)))
}
