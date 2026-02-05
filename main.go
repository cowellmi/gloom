package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hypnos"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensors"
)

type Platform interface {
	Now() time.Time
	Sleep(d time.Duration)
	// ReadConfig() Config
}

type Manager struct {
	sys     Platform
	logger  *log.Logger
	sensors []sensors.Sensor
}

func main() {
	var err error

	err = machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		println("I2C:", err)
		return
	}

	sys, err := hypnos.Probe(machine.I2C0)
	if err != nil {
		println("Hypnos:", err)
		return
	}

	err = machine.Serial.Configure(machine.UARTConfig{
		BaudRate: 115200,
	})
	if err != nil {
		println("Serial:", err)
		return
	}

	// Wait for serial connection
	for !machine.Serial.DTR() {
		time.Sleep(100 * time.Millisecond)
	}
	println("connected!")

	sensors := []sensors.Sensor{&sensors.Fake{}}

	logger := log.NewLogger(log.LevelDebug, true)

	man := Manager{
		sys:     sys,
		logger:  logger,
		sensors: sensors,
	}

	for {
		t := man.sys.Now()

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

		man.sys.Sleep(1 * time.Second)
	}
}
