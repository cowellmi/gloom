package config

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// RailConfig describes a single MOSFET-switched power rail: a GPIO
// pin number, its polarity, and which wake reasons activate it.
type RailConfig struct {
	Pin       uint8
	ActiveLow bool
	// SampleOnly marks this rail as only powered on during sample
	// wakes. Core rails (SampleOnly=false) are powered on every wake.
	SampleOnly bool
}

type Config struct {
	SampleInterval    time.Duration
	HeartbeatInterval time.Duration
	SerialEnabled     bool
	LedEnabled        bool
	Sensors           []string // raw IDs, resolved by caller

	// Power enables power rail control when non-empty. The value
	// selects a preset ("hypnos") whose default rails are set by
	// boardDefaults. Custom setups use any non-empty value (e.g.
	// "custom") with power_rails to specify pins explicitly.
	Power string

	// PowerRails lists MOSFET-switched power rails to control.
	// Set by boardDefaults for known boards; overridden by the
	// power_rails config key for custom wiring.
	PowerRails []RailConfig

	// SDCSPins lists chip-select pin numbers (as uint8) to probe
	// for an SD card, in priority order. Defaults cover Hypnos v3.3
	// (D11) and v3.2 (D10).
	SDCSPins []uint8

	// RTCWakePin is the GPIO pin number (as uint8) connected to the
	// RTC interrupt/alarm output. Used by the auto-prober to pass
	// to the RTC driver. Default is D12 (Hypnos standard wiring).
	RTCWakePin uint8
}

// DefaultINI is the default configuration file content, written to the
// SD card when no config.ini exists so the user has a documented
// template to edit.
const DefaultINI = `# Gloom configuration
#
# Lines starting with '#' are comments. Blank lines are ignored.
# All keys are optional; missing keys use built-in defaults.

# How often to wake and sample sensors. 0 = disabled.
# Accepts Go duration strings: "30s", "5m", "1h", etc.
sample_interval = 5s

# How often to send a heartbeat (keep-alive). 0 = disabled.
heartbeat_interval = 0

# Enable serial output (USB-CDC and UART).
serial = true

# Enable on-board LED blink on wake.
enable_led = true

# Comma-separated sensor IDs to sample each cycle.
# Must match IDs registered in the target's sensor registry.
sensors = fake

# Power rail control. Set to "hypnos" to use board-default rails,
# or any non-empty value (e.g. "custom") with power_rails below.
# Empty means no power management.
# power = hypnos

# MOSFET-switched power rails: comma-separated entries.
# Format: pin[:low][:sample]
#   - pin        GPIO pin number
#   - :low       active-low polarity (default is active-high)
#   - :sample    only powered on during sample wakes (saves power)
# Omit power_rails to use board defaults for the chosen preset.
# power_rails = 5:low, 6:sample

# Comma-separated SD card chip-select pin numbers to probe (in order).
# Board-specific defaults are applied automatically; override here if
# your wiring differs.
# sd_cs_pins = 16,18

# RTC alarm/interrupt pin number. Board-specific default is applied
# automatically; override here if your wiring differs.
# rtc_wake_pin = 19
`

// Default returns a Config with debug-friendly defaults. The fake
// sensor is included so a bare board with no config source still
// produces visible output on serial. When config is loaded from
// Blues Notecard or SD card, the sensors list is overridden.
//
// Board-specific defaults (pin numbers for SDCSPins, RTCWakePin, and
// Power) are not set here because they depend on machine.Pin values
// that are only available under TinyGo. Board files (e.g.
// main_feather_m0.go) apply those via boardDefaults() before any
// external config is loaded.
func Default() Config {
	return Config{
		SampleInterval:    5 * time.Second,
		HeartbeatInterval: 0,
		SerialEnabled:     true,
		LedEnabled:        true,
		Sensors:           []string{"fake"},
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

		case "serial":
			cfg.SerialEnabled = value == "true"

		case "enable_led":
			cfg.LedEnabled = value == "true"

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

		case "power":
			cfg.Power = value

		case "power_rails":
			rails, err := parseRailList(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.PowerRails = rails
			}

		case "sd_cs_pins":
			pins, err := parsePinList(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.SDCSPins = pins
			}

		case "rtc_wake_pin":
			pin, err := parsePin(key, value)
			if err != nil {
				errs = append(errs, err)
			} else {
				cfg.RTCWakePin = pin
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

// parsePinList parses a comma-separated list of numeric pin numbers.
func parsePinList(key, value string) ([]uint8, error) {
	var pins []uint8
	parts := strings.Split(value, ",")
	for _, p := range parts {
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

// parsePin parses a single numeric pin number (0–255).
func parsePin(key, value string) (uint8, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0, errors.New(key + ": invalid pin number: " + value)
	}
	return uint8(n), nil
}

// parseRailList parses a comma-separated list of rail entries.
// Format: pin[:low][:sample]
//   - pin is a GPIO pin number (required)
//   - :low sets active-low polarity (default is active-high)
//   - :sample marks the rail as sample-only
//
// Tags can appear in any order after the pin number.
// Examples: "5:low, 6:sample" or "5:low:sample" or "9"
func parseRailList(key, value string) ([]RailConfig, error) {
	var rails []RailConfig
	parts := strings.Split(value, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fields := strings.Split(p, ":")
		pin, err := parsePin(key, strings.TrimSpace(fields[0]))
		if err != nil {
			return nil, err
		}
		rc := RailConfig{Pin: pin}
		for _, f := range fields[1:] {
			switch strings.TrimSpace(f) {
			case "low":
				rc.ActiveLow = true
			case "sample":
				rc.SampleOnly = true
			default:
				return nil, errors.New(key + ": unknown rail tag: " + strings.TrimSpace(f))
			}
		}
		rails = append(rails, rc)
	}
	return rails, nil
}
