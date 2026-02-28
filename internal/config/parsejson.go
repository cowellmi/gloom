package config

import (
	"errors"

	"github.com/buger/jsonparser"
	"github.com/cowellmi/gloom/internal/hal"
)

// ParseJSON parses a JSON config from data into cfg.
// Top-level keys: led_pin, rtc_int_pin, sd_chip_select_pins, sinks, groups.
// Unknown top-level keys return an error.
// groups: clear-and-replace semantics.
// sinks: clear-and-replace semantics — cfg.Sinks is replaced entirely by whatever is in the JSON.
// After parsing, Validate is called to ensure every group has a wake source.
func ParseJSON(data []byte, cfg *Config) error {
	var errs []error

	if err := jsonparser.ObjectEach(data, func(key, value []byte, dataType jsonparser.ValueType, _ int) error {
		switch string(key) {
		case "led_pin":
			// Accept string ("17", "none") from MarshalMap or integer (17) from MarshalJSON.
			if dataType == jsonparser.String {
				s, _ := jsonparser.ParseString(value)
				pin, e := parsePin("led_pin", s)
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.LEDPin = pin
				}
			} else {
				n, e := jsonparser.ParseInt(value)
				if e != nil || n < 0 || n > 254 {
					errs = append(errs, errors.New("led_pin: invalid pin"))
				} else {
					cfg.LEDPin = hal.Pin(n)
				}
			}

		case "rtc_int_pin":
			// Accept string ("6") from MarshalMap or integer (6) from MarshalJSON.
			if dataType == jsonparser.String {
				s, _ := jsonparser.ParseString(value)
				pin, e := parsePin("rtc_int_pin", s)
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.RTCIntPin = pin
				}
			} else {
				n, e := jsonparser.ParseInt(value)
				if e != nil || n < 0 || n > 254 {
					errs = append(errs, errors.New("rtc_int_pin: invalid pin"))
				} else {
					cfg.RTCIntPin = hal.Pin(n)
				}
			}

		case "sd_chip_select_pins":
			// Accept string ("10, 11") from MarshalMap or array ([10, 11]) from MarshalJSON.
			cfg.SDChipSelectPins = cfg.SDChipSelectPins[:0]
			if dataType == jsonparser.String {
				s, _ := jsonparser.ParseString(value)
				pins, e := parsePinList("sd_chip_select_pins", s)
				if e != nil {
					errs = append(errs, e)
				} else {
					cfg.SDChipSelectPins = pins
				}
			} else {
				jsonparser.ArrayEach(value, func(v []byte, _ jsonparser.ValueType, _ int, _ error) {
					n, e := jsonparser.ParseInt(v)
					if e != nil || n < 0 || n > 254 {
						errs = append(errs, errors.New("sd_chip_select_pins: invalid pin"))
					} else {
						cfg.SDChipSelectPins = append(cfg.SDChipSelectPins, hal.Pin(n))
					}
				})
			}

		case "sinks":
			cfg.Sinks = make(map[string]SinkConfig)
			jsonparser.ObjectEach(value, func(sinkKey, sinkValue []byte, _ jsonparser.ValueType, _ int) error {
				name := string(sinkKey)
				var sc SinkConfig
				jsonparser.ObjectEach(sinkValue, func(fk, fv []byte, _ jsonparser.ValueType, _ int) error {
					switch string(fk) {
					case "log_level":
						s, _ := jsonparser.ParseString(fv)
						level, e := parseLevel(s)
						if e != nil {
							errs = append(errs, e)
						} else {
							sc.LogLevel = level
						}
					default:
						errs = append(errs, errors.New("sinks."+name+": unknown key: "+string(fk)))
					}
					return nil
				})
				cfg.Sinks[name] = sc
				return nil
			})

		case "groups":
			cfg.Groups = make(map[string]Group)
			jsonparser.ObjectEach(value, func(groupKey, groupValue []byte, _ jsonparser.ValueType, _ int) error {
				name := string(groupKey)
				var g Group
				jsonparser.ObjectEach(groupValue, func(fk, fv []byte, fdataType jsonparser.ValueType, _ int) error {
					switch string(fk) {
					case "interval":
						s, _ := jsonparser.ParseString(fv)
						d, e := parseDuration("interval", s)
						if e != nil {
							errs = append(errs, e)
						} else {
							g.Interval = d
						}
					case "sensors":
						// Accept string ("vbat, other") from MarshalMap or array from MarshalJSON.
						if fdataType == jsonparser.String {
							s, _ := jsonparser.ParseString(fv)
							g.Sensors = parseStringList(s)
						} else {
							jsonparser.ArrayEach(fv, func(v []byte, _ jsonparser.ValueType, _ int, _ error) {
								s, _ := jsonparser.ParseString(v)
								g.Sensors = append(g.Sensors, s)
							})
						}
					case "interrupt_pins":
						// Accept string ("6, 7") from MarshalMap or array from MarshalJSON.
						if fdataType == jsonparser.String {
							s, _ := jsonparser.ParseString(fv)
							pins, e := parsePinList("interrupt_pins", s)
							if e != nil {
								errs = append(errs, e)
							} else {
								g.InterruptPins = pins
							}
						} else {
							jsonparser.ArrayEach(fv, func(v []byte, _ jsonparser.ValueType, _ int, _ error) {
								n, e := jsonparser.ParseInt(v)
								if e != nil || n < 0 || n > 254 {
									errs = append(errs, errors.New("groups."+name+".interrupt_pins: invalid pin"))
								} else {
									g.InterruptPins = append(g.InterruptPins, hal.Pin(n))
								}
							})
						}
					case "pulse_led":
						// Accept string ("true") from MarshalMap or boolean from MarshalJSON.
						if fdataType == jsonparser.String {
							s, _ := jsonparser.ParseString(fv)
							switch s {
							case "true":
								g.PulseLED = true
							case "false":
								g.PulseLED = false
							default:
								errs = append(errs, errors.New("groups."+name+".pulse_led: expected true or false"))
							}
						} else {
							b, e := jsonparser.ParseBoolean(fv)
							if e != nil {
								errs = append(errs, errors.New("groups."+name+".pulse_led: "+e.Error()))
							} else {
								g.PulseLED = b
							}
						}
					default:
						errs = append(errs, errors.New("groups."+name+": unknown key: "+string(fk)))
					}
					return nil
				})
				cfg.Groups[name] = g
				return nil
			})

		default:
			errs = append(errs, errors.New("unknown key: "+string(key)))
		}
		return nil
	}); err != nil {
		errs = append(errs, err)
	}

	if err := Validate(cfg); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
