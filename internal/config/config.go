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

// LogSinkEntry pairs a sink name with its minimum log level.
// Parsed from the "name:level" syntax (e.g. "serial:debug").
// When no level is specified, LevelDebug is assumed.
type LogSinkEntry struct {
	Name  string
	Level log.Level
}

// Device holds user-configurable logging and data output settings,
// parsed from the [device] INI section. Hardware pin and rail
// configuration lives on the Board struct in the main package.
type Device struct {
	LogSinks  []LogSinkEntry
	DataSinks []string
	LedPin    hal.Pin
}

// Group defines a scheduled task with its own sensors and optional
// payload delivery. A group fires on a timer (Interval), an external
// interrupt (ExternalIntPin), or both.
type Group struct {
	Name           string
	Interval       time.Duration
	ExternalIntPin hal.Pin
	PulseLED       bool
	Sensors        []string
	Host           string
	Payload        Payload
}

// Config holds the complete parsed configuration.
type Config struct {
	Device Device
	Groups []Group
}

// Default returns a Config with debug-friendly defaults.
func Default() Config {
	return Config{
		Device: Device{
			LogSinks:  []LogSinkEntry{},
			DataSinks: []string{},
			LedPin:    hal.NoPin,
		},
		Groups: []Group{
			{
				Name:           "sample",
				Interval:       5 * time.Second,
				ExternalIntPin: hal.NoPin,
				Sensors:        []string{"vbat"},
				PulseLED:       true,
			},
		},
	}
}

// Parse reads an INI-format config from data into cfg. Sections map to
// [device] or named groups. All parse errors are collected and returned
// together via errors.Join so the caller can report every problem at once.
func Parse(data []byte, cfg *Config) error {
	var errs []error

	var (
		section    string
		groups     []Group
		groupIndex = make(map[string]int)
	)

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section == "" {
				errs = append(errs, errors.New("empty section name"))
			}
			if section != "device" {
				if _, exists := groupIndex[section]; !exists {
					groupIndex[section] = len(groups)
					groups = append(groups, Group{Name: section, ExternalIntPin: hal.NoPin})
				}
			}
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

		switch section {
		case "":
			errs = append(errs, errors.New("key outside of section: "+key))
		case "device":
			if err := parseDeviceKey(&cfg.Device, key, value); err != nil {
				errs = append(errs, err)
			}
		default:
			idx := groupIndex[section]
			if err := parseGroupKey(&groups[idx], key, value); err != nil {
				errs = append(errs, err)
			}
		}
	}

	cfg.Groups = groups

	for i := range cfg.Groups {
		if err := validateGroup(&cfg.Groups[i]); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func parseDeviceKey(dev *Device, key, value string) error {
	switch key {
	case "log_sinks":
		sinks, err := parseLogSinks(value)
		if err != nil {
			return err
		}
		dev.LogSinks = sinks
	case "data_sinks":
		names, err := parseDataSinks(value)
		if err != nil {
			return err
		}
		dev.DataSinks = names
	case "led_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		dev.LedPin = pin
	default:
		return errors.New("[device] unknown key: " + key)
	}
	return nil
}

func parseGroupKey(g *Group, key, value string) error {
	switch key {
	case "interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		g.Interval = d
	case "external_int_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		g.ExternalIntPin = pin
	case "pulse_led":
		g.PulseLED = parseBool(value)
	case "sensors":
		g.Sensors = parseStringList(value)
	case "host":
		g.Host = value
	case "payload":
		p, err := parsePayload(value)
		if err != nil {
			return err
		}
		g.Payload = p
	default:
		return errors.New("[" + g.Name + "] unknown key: " + key)
	}
	return nil
}

func validateGroup(g *Group) error {
	if g.Interval <= 0 && g.ExternalIntPin == hal.NoPin {
		return errors.New("[" + g.Name + "] must have interval or external_int_pin")
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
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return hal.NoPin, errors.New(key + ": invalid pin number: " + value)
	}
	return hal.Pin(n), nil
}
