package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// Payload identifies a predefined health-check payload profile.
type Payload uint8

const (
	PayloadNone Payload = iota
	PayloadMin
	PayloadFull
)

// Config holds the complete parsed configuration.
type Config struct {
	SDLogLevel        log.Level
	BluesLogLevel     log.Level
	SampleInterval    time.Duration
	SampleSensors     []string
	SampleExtPin      hal.Pin
	HeartbeatInterval time.Duration
	HeartbeatPayload  Payload
	HeartbeatLedPin   hal.Pin
}

// Default returns a Config seeded with board-supplied hardware defaults.
// ledPin is the board's LED pin (use hal.NoPin if absent).
// sensors is the board's default sensor list (may be nil).
// Sample is disabled by default (interval=0); heartbeat is enabled with a 3s interval.
func Default(ledPin hal.Pin, sensors []string) Config {
	return Config{
		SDLogLevel:        log.LevelDebug,
		BluesLogLevel:     log.LevelInfo,
		SampleInterval:    0,
		SampleSensors:     sensors,
		SampleExtPin:      hal.NoPin,
		HeartbeatInterval: 3 * time.Second,
		HeartbeatPayload:  PayloadNone,
		HeartbeatLedPin:   ledPin,
	}
}

// ParseINI reads a flat key=value config from data into cfg.
// Section headers (lines starting with '[') are silently skipped for
// backwards compatibility. All parse errors are collected and returned
// together via errors.Join so the caller can report every problem at once.
// After parsing, validates that at least one wake source is configured.
//
// ParseINI should be called on a Default()-initialised cfg so that fields
// absent from the file keep their sensible defaults (including NoPin sentinels).
func ParseINI(data []byte, cfg *Config) error {
	var errs []error

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip section headers (ignored in flat format).
		if strings.HasPrefix(line, "[") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip inline comments (require preceding space to avoid
		// truncating URLs with # fragments).
		if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}

		if err := parseKey(cfg, key, value); err != nil {
			errs = append(errs, err)
		}
	}

	if err := validate(cfg); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// ParseMap applies a Notecard env.get response body to cfg.
// Keys prefixed with '_' (Notehub-internal) are silently skipped.
// Empty string values are skipped (env var unset in Notehub).
// No structural validation is performed — the caller is responsible for
// ensuring the resulting cfg is sane (e.g. via the wake-source guard in main.go).
func ParseMap(cfg *Config, body map[string]interface{}) error {
	var errs []error
	for k, v := range body {
		if err := parseKey(cfg, k, v); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// parseKey dispatches a single key/value pair into cfg.
// v must be a string for all keys.
// Called by ParseINI (v is always string) and ParseMap (v may be a native type).
func parseKey(cfg *Config, key string, v interface{}) error {
	// Skip Notehub-internal keys.
	if strings.HasPrefix(key, "_") {
		return nil
	}

	value, ok := v.(string)
	if !ok {
		return errors.New(key + ": expected string")
	}

	// Skip empty values (unset Notehub env vars).
	if value == "" {
		return nil
	}

	switch key {
	case "sd_log_level":
		level, err := parseLevel(value)
		if err != nil {
			return err
		}
		cfg.SDLogLevel = level
	case "blues_log_level":
		level, err := parseLevel(value)
		if err != nil {
			return err
		}
		cfg.BluesLogLevel = level
	case "sample_interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		cfg.SampleInterval = d
	case "sample_sensors":
		cfg.SampleSensors = parseStringList(value)
	case "sample_ext_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.SampleExtPin = pin
	case "heartbeat_interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		cfg.HeartbeatInterval = d
	case "heartbeat_payload":
		p, err := parsePayload(value)
		if err != nil {
			return err
		}
		cfg.HeartbeatPayload = p
	case "heartbeat_led_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.HeartbeatLedPin = pin
	default:
		return errors.New("unknown key: " + key)
	}
	return nil
}

func validate(cfg *Config) error {
	if cfg.SampleInterval <= 0 && cfg.SampleExtPin == hal.NoPin && cfg.HeartbeatInterval <= 0 {
		return errors.New("config: no wake sources (needs sample_interval, sample_ext_pin, or heartbeat_interval)")
	}
	return nil
}

// --- parse helpers ---

func parseLevel(s string) (log.Level, error) {
	switch s {
	case "debug":
		return log.LevelDebug, nil
	case "info":
		return log.LevelInfo, nil
	case "warn":
		return log.LevelWarn, nil
	case "error":
		return log.LevelError, nil
	default:
		return 0, errors.New("unknown log level: " + s)
	}
}

func parsePayload(value string) (Payload, error) {
	switch value {
	case "none", "":
		return PayloadNone, nil
	case "min":
		return PayloadMin, nil
	case "full":
		return PayloadFull, nil
	default:
		return PayloadNone, errors.New("unknown payload: " + value)
	}
}

func parseStringList(value string) []string {
	var result []string
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		result = append(result, p)
	}
	return result
}

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

func parsePin(key, value string) (hal.Pin, error) {
	if strings.ToLower(value) == "none" {
		return hal.NoPin, nil
	}
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return hal.NoPin, errors.New(key + ": invalid pin number: " + value)
	}
	return hal.Pin(n), nil
}
