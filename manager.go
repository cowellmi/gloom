package main

import (
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
	"github.com/cowellmi/gloom/internal/hardware/fallback"
	"github.com/cowellmi/gloom/internal/hardware/hypnos"
	"github.com/cowellmi/gloom/internal/log"
)

type Manager struct {
	sys    hardware.Platform
	config Config
	logger *log.Logger
}

func NewManager() (*Manager, error) {
	var man Manager

	err := machine.Serial.Configure(machine.UARTConfig{
		BaudRate: 115200,
	})
	if err != nil {
		return nil, err
	}

	// Setup I2C
	err = machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		return nil, err
	}

	// Probe for platforms
	man.sys, err = hypnos.Probe(machine.I2C0)
	if err != nil {
		println("Hypnos:", err)
		println("Falling back.")
		man.sys = fallback.NewBoard()
	}

	// Parse config
	man.config = DefaultConfig()
	data, err := man.sys.ReadFile("config.txt")
	if err != nil {
		println("config: using defaults:", err)
	} else {
		err = ParseConfig(data, &man.config)
		if err != nil {
			println("config: parse error:", err)
		}
	}

	if man.config.WaitForSerial {
		for !machine.Serial.DTR() {
			time.Sleep(100 * time.Millisecond)
		}
	}

	man.logger = log.NewLogger(man.config.LogLevel, man.config.SerialEnabled)

	for _, s := range man.config.Sensors {
		if err := s.Init(); err != nil {
			msg := "init error for " + s.Name() + ": " + err.Error()
			man.Log(log.LevelError, msg)
		}
	}

	return &man, nil
}

func (man *Manager) Log(level log.Level, msg string) {
	t, err := man.sys.Now()
	if err != nil {
		t = time.Now()
		man.logger.Log(t, log.LevelError, "rtc: "+err.Error())
	}

	man.logger.Log(t, level, msg)
}
