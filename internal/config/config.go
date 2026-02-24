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

type SD struct {
	LogLevel log.Level
}

type Blues struct {
	LogLevel log.Level
}

// Sample defines the periodic or interrupt-driven sensor measurement schedule.
type Sample struct {
	Interval time.Duration
	Sensors  []string
	ExtPin   hal.Pin
}

// Heartbeat defines an optional keep-alive / payload delivery schedule.
// Interval = 0 disables the heartbeat entirely.
type Heartbeat struct {
	Interval time.Duration
	Payload  Payload
	LedPin   hal.Pin
}

// Config holds the complete parsed configuration.
type Config struct {
	SD        SD
	Blues     Blues
	Sample    Sample
	Heartbeat Heartbeat
}

// Default returns a Config with debug-friendly defaults.
// Sample is enabled with a 5s interval; heartbeat is disabled.
func Default(ledPin hal.Pin, sensors []string) Config {
	return Config{
		SD: SD{
			LogLevel: log.LevelDebug,
		},
		Blues: Blues{
			LogLevel: log.LevelInfo,
		},
		Sample: Sample{
			Interval: 9 * time.Second,
			Sensors:  sensors,
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			Interval: 3 * time.Second,
			Payload:  PayloadNone,
			LedPin:   ledPin,
		},
	}
}

// Parse reads a flat key=value config from data into cfg.
// Section headers (lines starting with '[') are silently skipped for
// backwards compatibility. All parse errors are collected and returned
// together via errors.Join so the caller can report every problem at once.
// Validation requires Sample to have Interval > 0 or ExtPin != NoPin.
//
// Parse should be called on a Default()-initialised cfg so that fields
// absent from the file keep their sensible defaults (including NoPin sentinels).
func Parse(data []byte, cfg *Config) error {
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

	if err := validateSample(&cfg.Sample); err != nil {
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
// v must be a string for all keys except blink_led, which also accepts bool.
// Called by Parse (v is always string) and ParseMap (v may be a native type).
func parseKey(cfg *Config, key string, v interface{}) error {
	value, ok := v.(string)
	if !ok {
		return errors.New(key + ": expected string")
	}

	switch key {
	case "log_sinks":
		sinks, err := parseLogSinks(value)
		if err != nil {
			return err
		}
		cfg.Device.LogSinks = sinks
	case "data_sinks":
		names, err := parseDataSinks(value)
		if err != nil {
			return err
		}
		cfg.Device.DataSinks = names
	case "led_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.Device.LedPin = pin
	case "interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		cfg.Sample.Interval = d
	case "sensors":
		cfg.Sample.Sensors = parseStringList(value)
	case "ext_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.Sample.ExtPin = pin
	case "heartbeat":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		cfg.Heartbeat.Interval = d
	case "payload":
		p, err := parsePayload(value)
		if err != nil {
			return err
		}
		cfg.Heartbeat.Payload = p
	default:
		return errors.New("unknown key: " + key)
	}
	return nil
}

func validateSample(s *Sample) error {
	if s.Interval <= 0 && s.ExtPin == hal.NoPin {
		return errors.New("sample: must have interval or ext_pin")
	}
	return nil
}

// --- parse helpers ---

var knownSinks = []string{"sd"}

func validSinkName(name string) bool {
	for _, s := range knownSinks {
		if s == name {
			return true
		}
	}
	return false
}

func parseLogSinks(value string) ([]LogSinkEntry, error) {
	var sinks []LogSinkEntry
	for _, p := range strings.Split(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		name, levelStr, hasLevel := strings.Cut(p, ":")
		name = strings.TrimSpace(name)
		if !validSinkName(name) {
			return nil, errors.New("unknown log sink: " + name)
		}
		level := log.LevelDebug
		if hasLevel {
			var err error
			level, err = parseLevel(strings.TrimSpace(levelStr))
			if err != nil {
				return nil, err
			}
		}
		sinks = append(sinks, LogSinkEntry{Name: name, Level: level})
	}
	return sinks, nil
}

func parseDataSinks(value string) ([]string, error) {
	var sinks []string
	for _, p := range strings.Split(value, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !validSinkName(p) {
			return nil, errors.New("unknown data sink: " + p)
		}
		sinks = append(sinks, p)
	}
	return sinks, nil
}

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

func parseBool(value string) bool {
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
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
