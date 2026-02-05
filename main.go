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

	sensors := []sensor.Sensor{}

	logger := log.NewLogger(true)

	clock, err := rtc.NewDS3231(machine.I2C0)
	if err != nil {
		println("DS3231:", err)
		return
	}

	man := manager{
		sensors: sensors,
		logger:  logger,
		clock:   clock,
	}

	for {
		t, err := man.clock.ReadTime()
		if err != nil {
			man.logger.Log(time.Time{}, log.LevelError, "can not read time from clock")
		} else {
			man.logger.Log(t, log.LevelInfo, "successfully read DS3231")
		}
		time.Sleep(1 * time.Second)
	}
}
