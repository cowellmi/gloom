package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

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
	case LogLevelOff:
		return "off"
	default:
		return strconv.FormatUint(uint64(l), 10)
	}
}

// SinkConfig holds per-sink configuration.
// Presence in Config.Sinks means the sink receives sensor data.
// LogLevel controls log output; LogLevelOff means data flows but no log output.
type SinkConfig struct {
	LogLevel LogLevel
}

// Group is a schedule group with its own interval, interrupt pins,
// sensor list, and LED pulse option. The name is the map key in Config.Groups.
type Group struct {
	Interval      time.Duration // 0 = no periodic wake
	InterruptPins []hal.Pin     // owned by this group only
	Sensors       []string      // sensor IDs
	PulseLED      bool
}

// Config holds the device-level configuration and schedule groups.
type Config struct {
	LEDPin           hal.Pin // status LED pin; hal.NoPin = none
	RTCIntPin        hal.Pin // RTC interrupt pin; hal.NoPin = use wing-detected pin
	SDChipSelectPins []hal.Pin
	Sinks            map[string]SinkConfig // keyed by "serial", "blues", "sd"
	Groups           map[string]Group      // keyed by group name
}

// Default returns a Config seeded with board-supplied hardware defaults.
// ledPin is the board's LED pin (use hal.NoPin if absent).
// rtcIntPin is the RTC interrupt pin detected by the wing (use hal.NoPin if absent).
// sensors is the board's default sensor list (may be nil).
// csPins is the list of SD card chip-select pins.
func Default(ledPin hal.Pin, rtcIntPin hal.Pin, sensors []string, csPins []hal.Pin) Config {
	return Config{
		LEDPin:           ledPin,
		RTCIntPin:        rtcIntPin,
		SDChipSelectPins: csPins,
		Sinks: map[string]SinkConfig{
			"serial": {LogLevel: LogLevelDebug},
			"sd":     {LogLevel: LogLevelError},
		},
		Groups: map[string]Group{
			"sample": {
				Interval: 5 * time.Second,
				Sensors:  sensors,
				PulseLED: true,
			},
		},
	}
}

// ParseMap applies a Notecard env.get response body to cfg.
// Keys prefixed with '_' (Notehub-internal) are silently skipped.
// Empty string values are skipped (env var unset in Notehub).
// The "groups" key must be a map[string]interface{} keyed by group name.
// On seeing a "groups" key, existing groups are cleared and replaced.
// The "sinks" key must be a map[string]interface{} keyed by sink name.
// On seeing a "sinks" key, cfg.Sinks is cleared and replaced entirely.
func ParseMap(cfg *Config, body map[string]interface{}) error {
	var errs []error
	for k, v := range body {
		if strings.HasPrefix(k, "_") {
			continue
		}

		if k == "groups" {
			gmap, ok := v.(map[string]interface{})
			if !ok {
				errs = append(errs, errors.New("groups: expected object"))
				continue
			}
			cfg.Groups = make(map[string]Group)
			for name, elem := range gmap {
				gdata, ok := elem.(map[string]interface{})
				if !ok {
					errs = append(errs, errors.New("groups."+name+": expected object"))
					continue
				}
				var g Group
				for gk, gv := range gdata {
					if gk == "pulse_led" {
						v, ok := gv.(bool)
						if !ok {
							errs = append(errs, errors.New("groups."+name+".pulse_led: expected bool"))
						} else {
							g.PulseLED = v
						}
						continue
					}
					value, ok := gv.(string)
					if !ok {
						errs = append(errs, errors.New("groups."+name+"."+gk+": expected string"))
						continue
					}
					if err := parseGroupKey(&g, gk, value); err != nil {
						errs = append(errs, err)
					}
				}
				cfg.Groups[name] = g
			}
			continue
		}

		if k == "sinks" {
			smap, ok := v.(map[string]interface{})
			if !ok {
				errs = append(errs, errors.New("sinks: expected object"))
				continue
			}
			cfg.Sinks = make(map[string]SinkConfig)
			for name, elem := range smap {
				sdata, ok := elem.(map[string]interface{})
				if !ok {
					errs = append(errs, errors.New("sinks."+name+": expected object"))
					continue
				}
				var sc SinkConfig
				for sk, sv := range sdata {
					value, ok := sv.(string)
					if !ok {
						errs = append(errs, errors.New("sinks."+name+"."+sk+": expected string"))
						continue
					}
					switch sk {
					case "log_level":
						level, err := parseLevel(value)
						if err != nil {
							errs = append(errs, err)
							continue
						}
						sc.LogLevel = level
					default:
						errs = append(errs, errors.New("sinks."+name+": unknown key: "+sk))
					}
				}
				cfg.Sinks[name] = sc
			}
			continue
		}

		value, ok := v.(string)
		if !ok {
			errs = append(errs, errors.New(k+": expected string"))
			continue
		}
		if value == "" {
			continue
		}
		if err := parseDeviceKey(cfg, k, value); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// parseDeviceKey dispatches a single device-scope key/value pair into cfg.
func parseDeviceKey(cfg *Config, key, value string) error {
	// Skip Notehub-internal keys.
	if strings.HasPrefix(key, "_") {
		return nil
	}

	// Skip empty values (unset Notehub env vars).
	if value == "" {
		return nil
	}

	switch key {
	case "led_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.LEDPin = pin
	case "rtc_int_pin":
		pin, err := parsePin(key, value)
		if err != nil {
			return err
		}
		cfg.RTCIntPin = pin
	case "sd_chip_select_pins":
		pins, err := parsePinList(key, value)
		if err != nil {
			return err
		}
		cfg.SDChipSelectPins = pins
	default:
		// Unknown keys are silently ignored to remain compatible with
		// configs written by older firmware versions.
	}
	return nil
}

// parseGroupKey dispatches a single group-scope key/value pair into g.
func parseGroupKey(g *Group, key, value string) error {
	if value == "" {
		return nil
	}

	switch key {
	case "interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		g.Interval = d
	case "interrupt_pins":
		pins, err := parsePinList(key, value)
		if err != nil {
			return err
		}
		g.InterruptPins = pins
	case "sensors":
		g.Sensors = parseStringList(value)
	default:
		// Unknown keys are silently ignored.
	}
	return nil
}

// Validate checks that every group has at least one wake source.
func Validate(cfg *Config) error {
	if len(cfg.Groups) == 0 {
		return errors.New("config: no groups defined")
	}
	for name, g := range cfg.Groups {
		if g.Interval == 0 && len(g.InterruptPins) == 0 {
			return errors.New("config: group \"" + name + "\" has no wake source (needs interval or interrupt_pins)")
		}
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
	case "off":
		return LogLevelOff, nil
	default:
		return 0, errors.New("unknown log level: " + s)
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
