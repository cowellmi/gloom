package hypnos

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/wait"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	// Number of attempts to retry I2C operations during probe.
	probeRetries = 5

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

func probeRTC(bus drivers.I2C) (*ds3231.Device, error) {
	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err := errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	// It may take a few tries to establish I2C connection after powering on.
	err := waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	// Clear any pending alarms.
	if err := rtc.ClearAlarm1(); err != nil {
		return nil, err
	}
	if err := rtc.ClearAlarm2(); err != nil {
		return nil, err
	}

	if err := rtc.SetSqwPinMode(ds3231.SQW_OFF); err != nil {
		return nil, err
	}

	return &rtc, nil
}

func waitForRTC(rtc *ds3231.Device) error {
	for attempt := 0; attempt < probeRetries; attempt++ {
		err := rtc.SetRunning(true)
		if err == nil {
			return nil
		}
		wait.For(probeRetryDelay)
	}

	return errors.New("hypnos: rtc communication timed out")
}
