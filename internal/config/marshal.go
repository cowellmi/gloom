package config

import (
	"errors"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// MarshalINI serializes the Config to flat key=value format.
// Only non-zero / non-NoPin fields are emitted. The output round-trips
// through ParseINI when called on a Default()-initialised destination.
func (c *Config) MarshalINI() ([]byte, error) {
	var buf []byte

	buf = append(buf, "# See example.config.ini for full documentation.\n"...)

	// SD
	sdLvl, err := levelString(c.SDLogLevel)
	if err != nil {
		return nil, err
	}
	buf = append(buf, "sd_log_level = "...)
	buf = append(buf, sdLvl...)
	buf = append(buf, '\n')

	// Blues
	bluesLvl, err := levelString(c.BluesLogLevel)
	if err != nil {
		return nil, err
	}
	buf = append(buf, "blues_log_level = "...)
	buf = append(buf, bluesLvl...)
	buf = append(buf, '\n')

	// Sample
	if c.SampleInterval > 0 {
		buf = append(buf, "sample_interval = "...)
		buf = appendDuration(buf, c.SampleInterval)
		buf = append(buf, '\n')
	}

	if len(c.SampleSensors) > 0 {
		buf = append(buf, "sample_sensors = "...)
		for i, s := range c.SampleSensors {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, s...)
		}
		buf = append(buf, '\n')
	}

	if c.SampleExtPin != hal.NoPin {
		buf = append(buf, "sample_ext_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.SampleExtPin), 10)
		buf = append(buf, '\n')
	}

	// Heartbeat
	if c.HeartbeatInterval > 0 {
		buf = append(buf, "heartbeat_interval = "...)
		buf = appendDuration(buf, c.HeartbeatInterval)
		buf = append(buf, '\n')
	}

	if c.HeartbeatPayload != PayloadNone {
		buf = append(buf, "heartbeat_payload = "...)
		ps, err := payloadString(c.HeartbeatPayload)
		if err != nil {
			return nil, err
		}
		buf = append(buf, ps...)
		buf = append(buf, '\n')
	}

	if c.HeartbeatLedPin != hal.NoPin {
		buf = append(buf, "heartbeat_led_pin = "...)
		buf = strconv.AppendUint(buf, uint64(c.HeartbeatLedPin), 10)
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

// MarshalMap serializes the Config to a map suitable for a Notecard note.add body.
func (c *Config) MarshalMap() map[string]interface{} {
	m := make(map[string]interface{})

	sdLvl, _ := levelString(c.SDLogLevel)
	m["sd_log_level"] = sdLvl

	bluesLvl, _ := levelString(c.BluesLogLevel)
	m["blues_log_level"] = bluesLvl

	if c.SampleInterval > 0 {
		m["sample_interval"] = durationString(c.SampleInterval)
	}

	if len(c.SampleSensors) > 0 {
		var s string
		for i, sensor := range c.SampleSensors {
			if i > 0 {
				s += ", "
			}
			s += sensor
		}
		m["sample_sensors"] = s
	}

	if c.SampleExtPin != hal.NoPin {
		m["sample_ext_pin"] = strconv.FormatUint(uint64(c.SampleExtPin), 10)
	} else {
		m["sample_ext_pin"] = "none"
	}

	if c.HeartbeatInterval > 0 {
		m["heartbeat_interval"] = durationString(c.HeartbeatInterval)
	}

	if c.HeartbeatPayload != PayloadNone {
		ps, _ := payloadString(c.HeartbeatPayload)
		m["heartbeat_payload"] = ps
	}

	if c.HeartbeatLedPin != hal.NoPin {
		m["heartbeat_led_pin"] = strconv.FormatUint(uint64(c.HeartbeatLedPin), 10)
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
