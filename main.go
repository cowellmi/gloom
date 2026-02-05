package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hypnos"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/rtc"
	"github.com/cowellmi/gloom/internal/sensor"
)

type manager struct {
	sensors []sensor.Sensor
	logger  *log.Logger
	clock   rtc.Clock
}

func main() {
	var err error

	err = machine.Serial.Configure(machine.UARTConfig{
		BaudRate: 115200,
	})
	if err != nil {
		println("Failed to configure Serial")
		return
	}

	// Wait for serial connection
	for !machine.Serial.DTR() {
		time.Sleep(100 * time.Millisecond)
	}
	println("connected!")

	hypnos.Configure()
	hypnos.PowerUp()
	time.Sleep(500 * time.Millisecond)

	err = machine.I2C0.Configure(machine.I2CConfig{}) // default config
	if err != nil {
		println("I2C:", err)
		return
	}

	sensors := []sensor.Sensor{&sensor.Fake{}}

	logger := log.NewLogger(log.LevelDebug, true)

	clock, err := rtc.NewDS3231(machine.I2C0)
	if err != nil {
		println("DS3231:", err)
	}

	man := manager{
		sensors: sensors,
		logger:  logger,
		clock:   clock,
	}

	for {
		t, err := man.clock.ReadTime()
		if err != nil {
			t = time.Now() // fallback to system clock
			msg := "failed to read time from RTC; falling back to system time"
			man.logger.Log(t, log.LevelWarn, msg)
		}

		for _, sen := range man.sensors {
			man.logger.Log(t, log.LevelDebug, sen.Name())
			ms, err := sen.Measure()
			if err != nil {
				msg := "failed to measure: " + sen.Name() + ": " + err.Error()
				man.logger.Log(t, log.LevelError, msg)
			}

			for _, m := range ms {
				msg := m.Label + ": " + m.Value + " " + m.Unit
				man.logger.Log(t, log.LevelDebug, msg)
			}
		}

		time.Sleep(1 * time.Second)
	}
}
