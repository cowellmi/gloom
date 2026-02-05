package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hypnos"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

type sleeper interface {
	Sleep() error
}

type manager struct {
	sensors []sensor.Sensor
	logger  *log.Logger
	sleeper sleeper
}

func main() {
	var err error

	err = machine.Serial.Configure(machine.UARTConfig{
		BaudRate: 115200,
	})
	if err != nil {
		println("Serial:", err)
		return
	}

	err = machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		println("I2C:", err)
		return
	}

	hypnos, err := hypnos.New(machine.I2C0)
	if err != nil {
		println("Hypnos:", err)
		return
	}

	// Wait for serial connection
	for !machine.Serial.DTR() {
		time.Sleep(100 * time.Millisecond)
	}
	println("connected!")

	sensors := []sensor.Sensor{&sensor.Fake{}}

	logger := log.NewLogger(log.LevelDebug, true)

	man := manager{
		sensors: sensors,
		logger:  logger,
	}

	for {
		t, err := hypnos.RTC.ReadTime()
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
