package config

import (
	"sort"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// MarshalJSON serializes the Config to compact human-readable JSON.
// One key per line, indented with spaces. Pins emitted as integers; omitted
// when NoPin. Sinks emitted in fixed order ["serial","blues","sd"]. Group
// names emitted sorted alphabetically. Only non-zero fields per group.
// Interval emitted as a string ("5s", "1m", "2h").
func (c *Config) MarshalJSON() ([]byte, error) {
	var buf []byte
	buf = append(buf, "{\n"...)
	first := true

	emit := func() {
		if !first {
			buf = append(buf, ",\n"...)
		}
		first = false
	}

	if c.LEDPin != hal.NoPin {
		emit()
		buf = append(buf, `  "led_pin": `...)
		buf = strconv.AppendUint(buf, uint64(c.LEDPin), 10)
	}

	if c.RTCIntPin != hal.NoPin {
		emit()
		buf = append(buf, `  "rtc_int_pin": `...)
		buf = strconv.AppendUint(buf, uint64(c.RTCIntPin), 10)
	}

	if len(c.SDChipSelectPins) > 0 {
		emit()
		buf = append(buf, `  "sd_chip_select_pins": [`...)
		for i, pin := range c.SDChipSelectPins {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = strconv.AppendUint(buf, uint64(pin), 10)
		}
		buf = append(buf, ']')
	}

	if len(c.Sinks) > 0 {
		emit()
		buf = append(buf, `  "sinks": {`...)
		sinkFirst := true
		for _, name := range []string{"serial", "blues", "sd"} {
			sc, ok := c.Sinks[name]
			if !ok {
				continue
			}
			if !sinkFirst {
				buf = append(buf, ',')
			}
			sinkFirst = false
			buf = append(buf, "\n    \""...)
			buf = append(buf, name...)
			buf = append(buf, "\": {\n      \"log_level\": \""...)
			buf = append(buf, sc.LogLevel.String()...)
			buf = append(buf, "\"\n    }"...)
		}
		buf = append(buf, "\n  }"...)
	}

	if len(c.Groups) > 0 {
		emit()
		buf = append(buf, `  "groups": {`...)
		names := make([]string, 0, len(c.Groups))
		for name := range c.Groups {
			names = append(names, name)
		}
		sort.Strings(names)

		groupFirst := true
		for _, name := range names {
			g := c.Groups[name]
			if !groupFirst {
				buf = append(buf, ',')
			}
			groupFirst = false
			buf = append(buf, "\n    \""...)
			buf = append(buf, name...)
			buf = append(buf, `": {`...)

			gFirst := true
			if g.Interval > 0 {
				buf = append(buf, "\n      \"interval\": \""...)
				buf = appendDuration(buf, g.Interval)
				buf = append(buf, '"')
				gFirst = false
			}
			if len(g.Sensors) > 0 {
				if !gFirst {
					buf = append(buf, ',')
				}
				gFirst = false
				buf = append(buf, "\n      \"sensors\": ["...)
				for i, s := range g.Sensors {
					if i > 0 {
						buf = append(buf, ", "...)
					}
					buf = append(buf, '"')
					buf = append(buf, s...)
					buf = append(buf, '"')
				}
				buf = append(buf, ']')
			}
			if len(g.InterruptPins) > 0 {
				if !gFirst {
					buf = append(buf, ',')
				}
				gFirst = false
				buf = append(buf, "\n      \"interrupt_pins\": ["...)
				for i, pin := range g.InterruptPins {
					if i > 0 {
						buf = append(buf, ", "...)
					}
					buf = strconv.AppendUint(buf, uint64(pin), 10)
				}
				buf = append(buf, ']')
			}
			if g.PulseLED {
				if !gFirst {
					buf = append(buf, ',')
				}
				buf = append(buf, "\n      \"pulse_led\": true"...)
			}
			buf = append(buf, "\n    }"...)
		}
		buf = append(buf, "\n  }"...)
	}

	buf = append(buf, "\n}\n"...)
	return buf, nil
}

// MarshalSerial returns the same JSON as MarshalJSON but with CRLF line
// endings, suitable for writing directly to a serial terminal.
func (c *Config) MarshalSerial() []byte {
	src, _ := c.MarshalJSON()
	out := make([]byte, 0, len(src)+16)
	for _, b := range src {
		if b == '\n' {
			out = append(out, '\r', '\n')
		} else {
			out = append(out, b)
		}
	}
	return out
}

// appendDuration writes a human-friendly duration.
// Emits clean single-unit forms when possible (5s, 3m, 2h) and
// falls back to time.Duration.String() for mixed durations (1m30s).
func appendDuration(buf []byte, d time.Duration) []byte {
	switch {
	case d%time.Hour == 0 && d >= time.Hour:
		buf = strconv.AppendInt(buf, int64(d/time.Hour), 10)
		return append(buf, 'h')
	case d%time.Minute == 0 && d >= time.Minute:
		buf = strconv.AppendInt(buf, int64(d/time.Minute), 10)
		return append(buf, 'm')
	case d%time.Second == 0 && d >= time.Second:
		buf = strconv.AppendInt(buf, int64(d/time.Second), 10)
		return append(buf, 's')
	default:
		return append(buf, d.String()...)
	}
}

// MarshalMap serializes the Config to a map suitable for a Notecard note.add body.
// Sinks are emitted as a nested map[string]interface{} under "sinks".
// Groups are emitted as a nested map[string]interface{} under "groups".
// led_pin is emitted as "none" when hal.NoPin.
func (c *Config) MarshalMap() map[string]interface{} {
	m := make(map[string]interface{})

	if c.LEDPin != hal.NoPin {
		m["led_pin"] = strconv.FormatUint(uint64(c.LEDPin), 10)
	} else {
		m["led_pin"] = "none"
	}

	if c.RTCIntPin != hal.NoPin {
		m["rtc_int_pin"] = strconv.FormatUint(uint64(c.RTCIntPin), 10)
	}

	if len(c.SDChipSelectPins) > 0 {
		var s string
		for i, pin := range c.SDChipSelectPins {
			if i > 0 {
				s += ", "
			}
			s += strconv.FormatUint(uint64(pin), 10)
		}
		m["sd_chip_select_pins"] = s
	}

	if len(c.Sinks) > 0 {
		sinks := make(map[string]interface{})
		for name, sc := range c.Sinks {
			sinks[name] = map[string]interface{}{"log_level": sc.LogLevel.String()}
		}
		m["sinks"] = sinks
	}

	if len(c.Groups) > 0 {
		groups := make(map[string]interface{})
		for name, g := range c.Groups {
			gmap := make(map[string]interface{})
			if g.Interval > 0 {
				gmap["interval"] = durationString(g.Interval)
			}
			if len(g.Sensors) > 0 {
				var s string
				for j, id := range g.Sensors {
					if j > 0 {
						s += ", "
					}
					s += id
				}
				gmap["sensors"] = s
			}
			if len(g.InterruptPins) > 0 {
				var s string
				for j, pin := range g.InterruptPins {
					if j > 0 {
						s += ", "
					}
					s += strconv.FormatUint(uint64(pin), 10)
				}
				gmap["interrupt_pins"] = s
			}
			if g.PulseLED {
				gmap["pulse_led"] = true
			}
			groups[name] = gmap
		}
		m["groups"] = groups
	}

	return m
}

func durationString(d time.Duration) string {
	switch {
	case d%time.Hour == 0 && d >= time.Hour:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d%time.Minute == 0 && d >= time.Minute:
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	case d%time.Second == 0 && d >= time.Second:
		return strconv.FormatInt(int64(d/time.Second), 10) + "s"
	default:
		return d.String()
	}
}
