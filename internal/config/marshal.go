package config

import (
	"errors"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// Marshal serializes the Config to flat key=value format.
// Only non-zero / non-NoPin fields are emitted. The output round-trips
// through Parse when called on a Default()-initialised destination.
func (c *Config) Marshal() ([]byte, error) {
	var buf []byte

	buf = append(buf, "# See example.config.ini for full documentation.\n"...)

	// Device keys
	if len(c.Device.LogSinks) > 0 {
		buf = append(buf, "log_sinks = "...)
		for i, s := range c.Device.LogSinks {
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

	if len(c.Device.DataSinks) > 0 {
		buf = append(buf, "data_sinks = "...)
		for i, s := range c.Device.DataSinks {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if c.Device.LedPin != hal.NoPin {
		buf = append(buf, "led_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.Device.LedPin), 10)
		buf = append(buf, '\n')
	}

	// Sample keys
	if c.Sample.Interval > 0 {
		buf = append(buf, "interval = "...)
		buf = appendDuration(buf, c.Sample.Interval)
		buf = append(buf, '\n')
	}

	if len(c.Sample.Sensors) > 0 {
		buf = append(buf, "sensors = "...)
		for i, s := range c.Sample.Sensors {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if c.Sample.ExtPin != hal.NoPin {
		buf = append(buf, "ext_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.Sample.ExtPin), 10)
		buf = append(buf, '\n')
	}

	// Heartbeat keys
	if c.Heartbeat.Interval > 0 {
		buf = append(buf, "heartbeat = "...)
		buf = appendDuration(buf, c.Heartbeat.Interval)
		buf = append(buf, '\n')
	}

	if c.Heartbeat.Payload != PayloadNone {
		buf = append(buf, "payload = "...)
		ps, err := payloadString(c.Heartbeat.Payload)
		if err != nil {
			return nil, err
		}
		buf = append(buf, ps...)
		buf = append(buf, '\n')
	}

	if c.Heartbeat.BlinkLED {
		buf = append(buf, "blink_led = true\n"...)
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
