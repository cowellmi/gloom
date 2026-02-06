package main

import (
	"errors"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensors"
)

type Config struct {
	SampleInterval    time.Duration
	HeartbeatInterval time.Duration
	SerialEnabled     bool
	WaitForSerial     bool
	LogLevel          log.Level
	Sensors           []sensors.Device
}

// Default values for debugging.
func DefaultConfig() Config {
	return Config{
		SampleInterval:    5 * time.Second,
		HeartbeatInterval: 0, // disabled
		SerialEnabled:     true,
		WaitForSerial:     true,
		LogLevel:          log.LevelDebug,
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
		case "sample_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			config.SampleInterval = d

		case "heartbeat_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			config.HeartbeatInterval = d

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
