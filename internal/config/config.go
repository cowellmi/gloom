package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// LogLevel represents log severity.
type LogLevel uint

const (
	LogLevelDebug LogLevel = 0
	LogLevelInfo  LogLevel = 1
	LogLevelWarn  LogLevel = 2
	LogLevelError LogLevel = 3
	LogLevelOff   LogLevel = 4
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	default:
		return strconv.FormatUint(uint64(l), 10)
	}
}

// HeartbeatPayload identifies a predefined heartbeat payload profile.
type HeartbeatPayload uint8

const (
	HeartbeatPayloadNone HeartbeatPayload = iota
	HeartbeatPayloadMin
	HeartbeatPayloadFull
)

// Config holds the complete parsed configuration.
type Config struct {
	HeartbeatInterval time.Duration
	HeartbeatLedPin   hal.Pin
	HeartbeatPayload  HeartbeatPayload
	InterruptPins     []hal.Pin
	LogLevelSD        LogLevel
	LogLevelBlues     LogLevel
	SampleInterval    time.Duration
	SampleSensors     []string
	SDChipSelectPins  []hal.Pin
}

// Default returns a Config seeded with board-supplied hardware defaults.
// ledPin is the board's LED pin (use hal.NoPin if absent).
// sensors is the board's default sensor list (may be nil).
// Sample is disabled by default (interval=0); heartbeat is enabled with a 3s interval.
func Default(ledPin hal.Pin, sensors []string, csPins []hal.Pin, interruptPins []hal.Pin) Config {
	return Config{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatLedPin:   ledPin,
		HeartbeatPayload:  HeartbeatPayloadNone,
		InterruptPins:     interruptPins,
		LogLevelBlues:     LogLevelOff,
		LogLevelSD:        LogLevelDebug,
		SampleInterval:    0, // disabled
		SampleSensors:     sensors,
		SDChipSelectPins:  csPins,
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
		cfg.LogLevelSD = level
	case "blues_log_level":
		level, err := parseLevel(value)
		if err != nil {
			return err
		}
		cfg.LogLevelBlues = level
	case "sample_interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		cfg.SampleInterval = d
	case "sample_sensors":
		cfg.SampleSensors = parseStringList(value)
	case "interrupt_pins":
		pins, err := parsePinList(key, value)
		if err != nil {
			return err
		}
		cfg.InterruptPins = pins
	case "sd_chip_select_pins":
		pins, err := parsePinList(key, value)
		if err != nil {
			return err
		}
		cfg.SDChipSelectPins = pins
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
	if cfg.SampleInterval <= 0 && len(cfg.InterruptPins) == 0 && cfg.HeartbeatInterval <= 0 {
		return errors.New("config: no wake sources (needs sample_interval, interrupt_pins, or heartbeat_interval)")
	}
	return nil
}

// --- parse helpers ---

func parseLevel(s string) (LogLevel, error) {
	switch s {
	case "debug":
		return LogLevelDebug, nil
	case "info":
		return LogLevelInfo, nil
	case "warn":
		return LogLevelWarn, nil
	case "error":
		return LogLevelError, nil
	default:
		return 0, errors.New("unknown log level: " + s)
	}
}

func parsePayload(value string) (HeartbeatPayload, error) {
	switch value {
	case "none", "":
		return HeartbeatPayloadNone, nil
	case "min":
		return HeartbeatPayloadMin, nil
	case "full":
		return HeartbeatPayloadFull, nil
	default:
		return HeartbeatPayloadNone, errors.New("unknown payload: " + value)
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

func parsePinList(key, value string) ([]hal.Pin, error) {
	var pins []hal.Pin
	for p := range strings.SplitSeq(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pin, err := parsePin(key, p)
		if err != nil {
			return nil, err
		}
		pins = append(pins, pin)
	}
	return pins, nil
}
