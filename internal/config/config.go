package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

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
// Parsed from the "name:level" syntax (e.g. "uart:debug").
// When no level is specified, LevelDebug is assumed.
type LogSinkEntry struct {
	Name  string
	Level log.Level
}

// RailConfig describes a MOSFET-switched power rail. Board files set
// defaults; the [rails] INI section can override or add rails.
type RailConfig struct {
	Name      string
	Pin       uint8
	ActiveLow bool
	Always    bool // true = on every wake; false = on-demand (sensors)
}

// Device holds hardware, logging, and data output configuration.
// These settings are not per-group — they describe the physical
// board and how all output is routed.
type Device struct {
	LogSinks   []LogSinkEntry
	DataSinks  []string
	LedPin     uint8
	SDCSPins   []uint8
	RTCWakePin uint8
	UARTTxPin  uint8
	UARTRxPin  uint8
	Rails      []RailConfig
}

// Group defines a scheduled task with its own sensors and optional
// payload delivery. A group fires on a timer (Interval), an external
// interrupt (ExternalIntPin), or both. Rails lists on-demand rail
// names this group requires when it has sensors.
type Group struct {
	Name           string
	Interval       time.Duration
	ExternalIntPin uint8
	PulseLED       bool
	Sensors        []string
	Rails          []string
	Host           string
	Payload        Payload
}

// Config holds the complete parsed configuration.
type Config struct {
	Device Device
	Groups []Group
}

// Default returns a Config with debug-friendly defaults. Board-specific
// pin defaults (SDCSPins, RTCWakePin, LedPin, UART pins) are not set
// here — board files apply those via initBoard() before any external
// config is loaded.
func Default() Config {
	return Config{
		Device: Device{
			LogSinks: []LogSinkEntry{
				{Name: "uart", Level: log.LevelDebug},
				{Name: "usb", Level: log.LevelDebug},
			},
			DataSinks: []string{"uart", "usb"},
		},
		Groups: []Group{
			{
				Name:     "sample",
				Interval: 5 * time.Second,
				Sensors:  []string{"fake"},
				PulseLED: true,
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
		section      string
		groups       []Group
		groupIndex   = make(map[string]int)
		railsCleared bool
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
			if section != "device" && section != "rails" {
				if _, exists := groupIndex[section]; !exists {
					groupIndex[section] = len(groups)
					groups = append(groups, Group{Name: section})
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

		// Strip inline comments.
		if i := strings.IndexByte(value, '#'); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}

		switch section {
		case "":
			errs = append(errs, errors.New("key outside of section: "+key))
		case "device":
			if err := parseDeviceKey(&cfg.Device, key, value); err != nil {
				errs = append(errs, err)
			}
		case "rails":
			if !railsCleared {
				cfg.Device.Rails = nil
				railsCleared = true
			}
			rc, err := parseRailValue(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.Device.Rails = append(cfg.Device.Rails, rc)
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
		if err := validateGroup(&cfg.Groups[i], &cfg.Device); err != nil {
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
	case "rtc_wake_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		dev.RTCWakePin = pin
	case "uart_tx_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		dev.UARTTxPin = pin
	case "uart_rx_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		dev.UARTRxPin = pin
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
	case "rails":
		g.Rails = parseStringList(value)
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

func validateGroup(g *Group, dev *Device) error {
	var errs []error
	if g.Interval <= 0 && g.ExternalIntPin == 0 {
		errs = append(errs, errors.New("["+g.Name+"] must have interval or external_int_pin"))
	}
	if len(g.Sensors) > 0 && len(dev.DataSinks) == 0 {
		errs = append(errs, errors.New("["+g.Name+"] sensors require at least one data_sink in [device]"))
	}
	for _, rn := range g.Rails {
		found := false
		for _, rc := range dev.Rails {
			if rc.Name == rn {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, errors.New("["+g.Name+"] unknown rail: "+rn))
		}
	}
	return errors.Join(errs...)
}

// --- parse helpers ---

var knownSinks = []string{"uart", "usb", "sd"}

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
	for _, p := range strings.Split(value, ",") {
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

func parsePinList(key, value string) ([]uint8, error) {
	var pins []uint8
	for _, p := range strings.Split(value, ",") {
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

// parseRailValue parses a rail definition: "pin, polarity[, always]".
// The key becomes the rail name. Example: "5v = 6, high" or
// "3v3 = 5, low, always".
// parseRailValue parses a rail definition: "pin[, polarity][, always]".
// Polarity defaults to high (active-high). The key becomes the rail name.
// Examples: "5v = 20", "3v3 = 15, low, always".
func parseRailValue(name, value string) (RailConfig, error) {
	parts := strings.Split(value, ",")
	if len(parts) < 1 || len(parts) > 3 {
		return RailConfig{}, errors.New("[rails] " + name + ": expected pin[, low|high][, always]")
	}

	pinStr := strings.TrimSpace(parts[0])
	pin, err := strconv.ParseUint(pinStr, 10, 8)
	if err != nil {
		return RailConfig{}, errors.New("[rails] " + name + ": invalid pin number: " + pinStr)
	}

	var activeLow bool
	always := false

	for _, part := range parts[1:] {
		switch strings.TrimSpace(part) {
		case "low":
			activeLow = true
		case "high":
			activeLow = false
		case "always":
			always = true
		default:
			return RailConfig{}, errors.New("[rails] " + name + ": unknown option: " + strings.TrimSpace(part))
		}
	}

	return RailConfig{
		Name:      name,
		Pin:       uint8(pin),
		ActiveLow: activeLow,
		Always:    always,
	}, nil
}

func parsePin(key, value string) (uint8, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, errors.New(key + ": invalid pin number: " + value)
	}
	return uint8(n), nil
}
