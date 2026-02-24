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
func (c *Config) MarshalINI() ([]byte, error) {
	var buf []byte

	buf = append(buf, "# See example.config.ini for full documentation.\n"...)

	// SD
	sdLvl, err := levelString(c.SD.LogLevel)
	if err != nil {
		return nil, err
	}
	buf = append(buf, "sd_log_level = "...)
	buf = append(buf, sdLvl...)
	buf = append(buf, '\n')

	// Blues
	bluesLvl, err := levelString(c.Blues.LogLevel)
	if err != nil {
		return nil, err
	}
	buf = append(buf, "blues_log_level = "...)
	buf = append(buf, bluesLvl...)
	buf = append(buf, '\n')

	// Sample
	if c.Sample.Interval > 0 {
		buf = append(buf, "sample_interval = "...)
		buf = appendDuration(buf, c.Sample.Interval)
		buf = append(buf, '\n')
	}

	if len(c.Sample.Sensors) > 0 {
		buf = append(buf, "sample_sensors = "...)
		for i, s := range c.Sample.Sensors {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if c.Sample.ExtPin != hal.NoPin {
		buf = append(buf, "sample_ext_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.Sample.ExtPin), 10)
		buf = append(buf, '\n')
	}

	// Heartbeat
	if c.Heartbeat.Interval > 0 {
		buf = append(buf, "heartbeat_interval = "...)
		buf = appendDuration(buf, c.Heartbeat.Interval)
		buf = append(buf, '\n')
	}

	if c.Heartbeat.Payload != PayloadNone {
		buf = append(buf, "heartbeat_payload = "...)
		ps, err := payloadString(c.Heartbeat.Payload)
		if err != nil {
			return nil, err
		}
		buf = append(buf, ps...)
		buf = append(buf, '\n')
	}

	if c.Heartbeat.LedPin != hal.NoPin {
		buf = append(buf, "heartbeat_led_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.Heartbeat.LedPin), 10)
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

// MarshalMap serializes the Config to a map suitable for a Notecard env.update body.
func (c *Config) MarshalMap() map[string]interface{} {
	m := make(map[string]interface{})

	sdLvl, _ := levelString(c.SD.LogLevel)
	m["sd_log_level"] = sdLvl

	bluesLvl, _ := levelString(c.Blues.LogLevel)
	m["blues_log_level"] = bluesLvl

	if c.Sample.Interval > 0 {
		m["sample_interval"] = durationString(c.Sample.Interval)
	}

	if len(c.Sample.Sensors) > 0 {
		var s string
		for i, sensor := range c.Sample.Sensors {
			if i > 0 {
				s += ", "
			}
			s += sensor
		}
		m["sample_sensors"] = s
	}

	if c.Sample.ExtPin != hal.NoPin {
		m["sample_ext_pin"] = strconv.FormatUint(uint64(c.Sample.ExtPin), 10)
	} else {
		m["sample_ext_pin"] = "none"
	}

	if c.Heartbeat.Interval > 0 {
		m["heartbeat_interval"] = durationString(c.Heartbeat.Interval)
	}

	if c.Heartbeat.Payload != PayloadNone {
		ps, _ := payloadString(c.Heartbeat.Payload)
		m["heartbeat_payload"] = ps
	}

	if c.Heartbeat.LedPin != hal.NoPin {
		m["heartbeat_led_pin"] = strconv.FormatUint(uint64(c.Heartbeat.LedPin), 10)
	} else {
		m["heartbeat_led_pin"] = "none"
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
