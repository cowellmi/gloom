package config

import (
	"errors"

	"github.com/andreyvit/tinyjson"
	"github.com/cowellmi/gloom/internal/hal"
)

// ParseJSON parses a JSON config from data into cfg.
// Top-level keys: led_pin, rtc_int_pin, sd_chip_select_pins, sinks, groups.
// Unknown top-level keys return an error.
// groups: clear-and-replace semantics.
// sinks: merge semantics — only update sinks already in cfg.Sinks.
// After parsing, Validate is called to ensure every group has a wake source.
//
// tinyjson panics on malformed JSON; those panics are caught and returned as errors.
func ParseJSON(data []byte, cfg *Config) (parseErr error) {
	defer func() {
		if r := recover(); r != nil {
			switch v := r.(type) {
			case error:
				parseErr = errors.New("config: json: " + v.Error())
			case string:
				parseErr = errors.New("config: json: " + v)
			default:
				parseErr = errors.New("config: json: parse error")
			}
		}
	}()

	var errs []error
	raw := tinyjson.Raw(data)

	for key := raw.StartObject(); key != nil; key = raw.ContinueObject() {
		switch key.Str() {
		case "led_pin":
			// Accept string ("17", "none") from MarshalMap or integer (17) from MarshalJSON.
			if raw.Peek() == tinyjson.String {
				pin, e := parsePin("led_pin", raw.Str())
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.LEDPin = pin
				}
			} else {
				n := raw.Int()
				if n < 0 || n > 254 {
					errs = append(errs, errors.New("led_pin: invalid pin"))
				} else {
					cfg.LEDPin = hal.Pin(n)
				}
			}

		case "rtc_int_pin":
			// Accept string ("6") from MarshalMap or integer (6) from MarshalJSON.
			if raw.Peek() == tinyjson.String {
				pin, e := parsePin("rtc_int_pin", raw.Str())
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.RTCIntPin = pin
				}
			} else {
				n := raw.Int()
				if n < 0 || n > 254 {
					errs = append(errs, errors.New("rtc_int_pin: invalid pin"))
				} else {
					cfg.RTCIntPin = hal.Pin(n)
				}
			}

		case "sd_chip_select_pins":
			// Accept string ("10, 11") from MarshalMap or array ([10, 11]) from MarshalJSON.
			cfg.SDChipSelectPins = cfg.SDChipSelectPins[:0]
			if raw.Peek() == tinyjson.String {
				pins, e := parsePinList("sd_chip_select_pins", raw.Str())
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.SDChipSelectPins = pins
				}
			} else {
				for raw.StartArray(); raw.ContinueArray(); {
					n := raw.Int()
					if n < 0 || n > 254 {
						errs = append(errs, errors.New("sd_chip_select_pins: invalid pin"))
					} else {
						cfg.SDChipSelectPins = append(cfg.SDChipSelectPins, hal.Pin(n))
					}
				}
			}

		case "sinks":
			for sinkKey := raw.StartObject(); sinkKey != nil; sinkKey = raw.ContinueObject() {
				name := sinkKey.Str()
				if _, exists := cfg.Sinks[name]; !exists {
					raw.Skip()
					continue
				}
				sc := cfg.Sinks[name]
				for fieldKey := raw.StartObject(); fieldKey != nil; fieldKey = raw.ContinueObject() {
					switch fieldKey.Str() {
					case "log_level":
						level, e := parseLevel(raw.Str())
						if e != nil {
							errs = append(errs, e)
						} else {
							sc.LogLevel = level
						}
					default:
						errs = append(errs, errors.New("sinks."+name+": unknown key: "+fieldKey.Str()))
						raw.Skip()
					}
				}
				cfg.Sinks[name] = sc
			}

		case "groups":
			cfg.Groups = make(map[string]Group)
			for groupKey := raw.StartObject(); groupKey != nil; groupKey = raw.ContinueObject() {
				name := groupKey.Str()
				var g Group
				for fieldKey := raw.StartObject(); fieldKey != nil; fieldKey = raw.ContinueObject() {
					switch fieldKey.Str() {
					case "interval":
						d, e := parseDuration("interval", raw.Str())
						if e != nil {
							errs = append(errs, e)
						} else {
							g.Interval = d
						}
					case "sensors":
						// Accept string ("vbat, other") from MarshalMap or array from MarshalJSON.
						if raw.Peek() == tinyjson.String {
							g.Sensors = parseStringList(raw.Str())
						} else {
							for raw.StartArray(); raw.ContinueArray(); {
								g.Sensors = append(g.Sensors, raw.Str())
							}
						}
					case "interrupt_pins":
						// Accept string ("6, 7") from MarshalMap or array from MarshalJSON.
						if raw.Peek() == tinyjson.String {
							pins, e := parsePinList("interrupt_pins", raw.Str())
							if e != nil {
								errs = append(errs, e)
							} else {
								g.InterruptPins = pins
							}
						} else {
							for raw.StartArray(); raw.ContinueArray(); {
								n := raw.Int()
								if n < 0 || n > 254 {
									errs = append(errs, errors.New("groups."+name+".interrupt_pins: invalid pin"))
								} else {
									g.InterruptPins = append(g.InterruptPins, hal.Pin(n))
								}
							}
						}
					case "pulse_led":
						// Accept string ("true") from MarshalMap or boolean from MarshalJSON.
						if raw.Peek() == tinyjson.String {
							switch raw.Str() {
							case "true":
								g.PulseLED = true
							case "false":
								g.PulseLED = false
							default:
								errs = append(errs, errors.New("groups."+name+".pulse_led: expected true or false"))
							}
						} else {
							g.PulseLED = raw.Bool()
						}
					default:
						errs = append(errs, errors.New("groups."+name+": unknown key: "+fieldKey.Str()))
						raw.Skip()
					}
				}
				cfg.Groups[name] = g
			}

		default:
			errs = append(errs, errors.New("unknown key: "+key.Str()))
			raw.Skip()
		}
	}

	if err := Validate(cfg); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
