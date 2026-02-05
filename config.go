package main

import (
	"errors"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensors"
	"github.com/cowellmi/gloom/internal/sensors/fake"
)

type Config struct {
	SleepInterval time.Duration
	SerialEnabled bool
	WaitForSerial bool
	LogLevel      log.Level
	Sensors       []sensors.Device
}

var sensorRegistry = map[string]func() sensors.Device{
	"fake": func() sensors.Device { return fake.NewDevice() },
}

func DefaultConfig() Config {
	return Config{
		SleepInterval: 60 * time.Second,
		SerialEnabled: true,
		WaitForSerial: true,
		LogLevel:      log.LevelInfo,
	}
}

func ParseConfig(data []byte, config *Config) error {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "sleep_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			config.SleepInterval = d

		case "serial":
			config.SerialEnabled = value == "true"

		case "wait_for_serial":
			config.WaitForSerial = value == "true"

		case "log_level":
			switch value {
			case "debug":
				config.LogLevel = log.LevelDebug
			case "info":
				config.LogLevel = log.LevelInfo
			case "warn":
				config.LogLevel = log.LevelWarn
			case "error":
				config.LogLevel = log.LevelError
			}

		case "sensors":
			ids := strings.Split(value, ",")
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				newDevice, ok := sensorRegistry[id]
				if !ok {
					return errors.New("unknown sensor: " + id)
				}
				config.Sensors = append(config.Sensors, newDevice())
			}
		}
	}

	return nil
}
