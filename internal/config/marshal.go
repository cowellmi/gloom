package config

import (
	"errors"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// Marshal serializes the Config to INI format. Only non-zero fields
// are emitted. The output round-trips through Parse.
func (c *Config) Marshal() ([]byte, error) {
	var buf []byte

	buf = append(buf, "# See example.config.ini for full documentation.\n"...)

	// [device]
	var err error
	buf, err = appendDevice(buf, &c.Device)
	if err != nil {
		return nil, err
	}

	// groups
	for i := range c.Groups {
		var gerr error
		buf, gerr = appendGroup(buf, &c.Groups[i])
		if gerr != nil {
			return nil, gerr
		}
	}

	return buf, nil
}

func appendDevice(buf []byte, dev *Device) ([]byte, error) {
	buf = append(buf, "\n[device]\n"...)

	if len(dev.LogSinks) > 0 {
		buf = append(buf, "log_sinks = "...)
		for i, s := range dev.LogSinks {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s.Name...)
			buf = append(buf, ':')
			ls, err := levelString(s.Level)
			if err != nil {
				return nil, err
			}
			buf = append(buf, ls...)
		}
		buf = append(buf, '\n')
	}

	if len(dev.DataSinks) > 0 {
		buf = append(buf, "data_sinks = "...)
		for i, s := range dev.DataSinks {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if dev.LedPin != hal.NoPin {
		buf = append(buf, "led_pin = "...)
		buf = strconv.AppendUint(buf, uint64(dev.LedPin), 10)
		buf = append(buf, '\n')
	}

	return buf, nil
}

func appendGroup(buf []byte, g *Group) ([]byte, error) {
	buf = append(buf, '\n')
	buf = append(buf, '[')
	buf = append(buf, g.Name...)
	buf = append(buf, "]\n"...)

	if g.Interval > 0 {
		buf = append(buf, "interval = "...)
		buf = appendDuration(buf, g.Interval)
		buf = append(buf, '\n')
	}

	if g.ExternalIntPin != hal.NoPin {
		buf = append(buf, "external_int_pin = "...)
		buf = strconv.AppendUint(buf, uint64(g.ExternalIntPin), 10)
		buf = append(buf, '\n')
	}

	if len(g.Sensors) > 0 {
		buf = append(buf, "sensors = "...)
		for i, s := range g.Sensors {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if g.PulseLED {
		buf = append(buf, "pulse_led = true\n"...)
	}

	if g.Host != "" {
		buf = append(buf, "host = "...)
		buf = append(buf, g.Host...)
		buf = append(buf, '\n')
	}

	if g.Payload != PayloadNone {
		buf = append(buf, "payload = "...)
		ps, err := payloadString(g.Payload)
		if err != nil {
			return nil, err
		}
		buf = append(buf, ps...)
		buf = append(buf, '\n')
	}

	return buf, nil
}

// appendDuration writes a human-friendly duration. Go's
// time.Duration.String() works but produces "1m0s" instead of "1m".
// This emits clean single-unit forms when possible (5s, 3m, 2h) and
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

func levelString(l log.Level) (string, error) {
	switch l {
	case log.LevelDebug:
		return "debug", nil
	case log.LevelInfo:
		return "info", nil
	case log.LevelWarn:
		return "warn", nil
	case log.LevelError:
		return "error", nil
	default:
		return "", errors.New("unknown log level: " + strconv.Itoa(int(l)))
	}
}

func payloadString(p Payload) (string, error) {
	switch p {
	case PayloadNone:
		return "none", nil
	case PayloadMin:
		return "min", nil
	case PayloadFull:
		return "full", nil
	default:
		return "", errors.New("unknown payload: " + strconv.Itoa(int(p)))
	}
}
