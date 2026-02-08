package config

import (
	"errors"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/log"
)

type Config struct {
	SampleInterval    time.Duration
	HeartbeatInterval time.Duration
	SerialEnabled     bool
	WaitForSerial     bool
	MaxWaitForSerial  time.Duration
	LogLevel          log.Level
	EnableMachineLED  bool
	Sensors           []string // raw IDs, resolved by caller
}

// Default returns a Config with debug-friendly defaults.
func Default() Config {
	return Config{
		SampleInterval:    5 * time.Second,
		HeartbeatInterval: 0, // disabled
		SerialEnabled:     true,
		WaitForSerial:     true,
		MaxWaitForSerial:  5 * time.Minute,
		LogLevel:          log.LevelDebug,
		EnableMachineLED:  true,
	}
}

// Parse reads a key=value config from data into cfg. Lines starting
// with '#' and blank lines are ignored. All parse errors are collected
// and returned together via errors.Join so the caller can report every
// problem at once (useful on headless devices where re-flashing is expensive).
func Parse(data []byte, cfg *Config) error {
	var errs []error

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
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
			d, err := parseDuration(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.SampleInterval = d
			}

		case "heartbeat_interval":
			d, err := parseDuration(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.HeartbeatInterval = d
			}

		case "max_wait_for_serial":
			d, err := parseDuration(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.MaxWaitForSerial = d
			}

		case "serial":
			cfg.SerialEnabled = value == "true"

		case "wait_for_serial":
			cfg.WaitForSerial = value == "true"

		case "enable_led":
			cfg.EnableMachineLED = value == "true"

		case "log_level":
			switch value {
			case "debug":
				cfg.LogLevel = log.LevelDebug
			case "info":
				cfg.LogLevel = log.LevelInfo
			case "warn":
				cfg.LogLevel = log.LevelWarn
			case "error":
				cfg.LogLevel = log.LevelError
			default:
				errs = append(errs, errors.New("unknown log_level: "+value))
			}

		case "sensors":
			// Reset before appending so re-parsing doesn't accumulate duplicates.
			cfg.Sensors = cfg.Sensors[:0]
			ids := strings.Split(value, ",")
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				cfg.Sensors = append(cfg.Sensors, id)
			}

		default:
			errs = append(errs, errors.New("unknown config key: "+key))
		}
	}

	return errors.Join(errs...)
}

// parseDuration parses a duration string and rejects negative values.
// Zero is permitted for fields where it means "disabled."
func parseDuration(key, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, errors.New(key + ": negative duration not allowed: " + value)
	}
	return d, nil
}
