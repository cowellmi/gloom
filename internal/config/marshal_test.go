package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

func TestMarshal_RoundTrip(t *testing.T) {
	orig := Config{
		Device: Device{
			LogSinks: []LogSinkEntry{
				{Name: "sd", Level: log.LevelError},
			},
			DataSinks: []string{"sd"},
			LedPin:    hal.NoPin,
		},
		Sample: Sample{
			Interval: time.Minute,
			Sensors:  []string{"temperature", "humidity"},
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			Interval: time.Hour,
			Host:     "http://example.com/hb",
			Payload:  PayloadFull,
		},
	}

	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// Start from Default so NoPin sentinels are correctly seeded for
	// fields absent from the marshaled output.
	got := Default()
	if err := Parse(data, &got); err != nil {
		t.Fatalf("Parse(Marshal()) error: %v\nINI:\n%s", err, data)
	}

	// Device
	if len(got.Device.LogSinks) != 1 {
		t.Errorf("LogSinks = %d, want 1", len(got.Device.LogSinks))
	}
	if got.Device.LogSinks[0].Name != "sd" || got.Device.LogSinks[0].Level != log.LevelError {
		t.Errorf("LogSink[0] = %+v", got.Device.LogSinks[0])
	}
	if len(got.Device.DataSinks) != 1 || got.Device.DataSinks[0] != "sd" {
		t.Errorf("DataSinks = %v", got.Device.DataSinks)
	}

	// Sample
	if got.Sample.Interval != time.Minute {
		t.Errorf("Sample.Interval = %v, want 1m", got.Sample.Interval)
	}
	if len(got.Sample.Sensors) != 2 {
		t.Errorf("Sample.Sensors = %v", got.Sample.Sensors)
	}
	if got.Sample.ExtPin != hal.NoPin {
		t.Errorf("Sample.ExtPin = %d, want NoPin", got.Sample.ExtPin)
	}

	// Heartbeat
	if got.Heartbeat.Interval != time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want 1h", got.Heartbeat.Interval)
	}
	if got.Heartbeat.Host != "http://example.com/hb" {
		t.Errorf("Heartbeat.Host = %q", got.Heartbeat.Host)
	}
	if got.Heartbeat.Payload != PayloadFull {
		t.Errorf("Heartbeat.Payload = %d, want PayloadFull", got.Heartbeat.Payload)
	}
}

func TestMarshal_ZeroFieldsOmitted(t *testing.T) {
	cfg := Config{
		Device: Device{
			LedPin: hal.NoPin,
		},
		Sample: Sample{
			Interval: 5 * time.Second,
			ExtPin:   hal.NoPin,
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	s := string(data)
	for _, absent := range []string{
		"sensors", "blink_led", "host", "payload",
		"ext_pin", "heartbeat", "log_sinks", "data_sinks", "led_pin",
	} {
		if strings.Contains(s, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, s)
		}
	}

	if !strings.Contains(s, "interval = 5s") {
		t.Errorf("output should contain 'interval = 5s':\n%s", s)
	}
}

func TestMarshal_ExtPin(t *testing.T) {
	cfg := Config{
		Sample: Sample{
			ExtPin:  hal.Pin(7),
			Sensors: []string{"bucket"},
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if !strings.Contains(string(data), "ext_pin = 7") {
		t.Errorf("expected ext_pin = 7 in output:\n%s", data)
	}
}

func TestMarshal_HeartbeatDisabled(t *testing.T) {
	cfg := Config{
		Sample: Sample{
			Interval: 5 * time.Second,
			ExtPin:   hal.NoPin,
		},
		// Heartbeat.Interval = 0 → disabled
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if strings.Contains(string(data), "heartbeat") {
		t.Errorf("output should not contain 'heartbeat' when disabled:\n%s", data)
	}
}

func TestMarshal_DurationFormatting(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{time.Minute, "1m"},
		{2 * time.Hour, "2h"},
		{90 * time.Second, "90s"},
	}

	for _, tt := range tests {
		cfg := Config{
			Sample: Sample{
				Interval: tt.d,
				ExtPin:   hal.NoPin,
			},
		}
		data, err := cfg.Marshal()
		if err != nil {
			t.Fatalf("Marshal() error for %v: %v", tt.d, err)
		}
		if !strings.Contains(string(data), "interval = "+tt.want) {
			t.Errorf("duration %v: want %q in output:\n%s", tt.d, tt.want, data)
		}
	}
}

func TestMarshal_BlinkLED(t *testing.T) {
	cfg := Config{
		Sample: Sample{
			Interval: time.Minute,
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			Interval: time.Hour,
			BlinkLED: true,
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if !strings.Contains(string(data), "blink_led = true") {
		t.Errorf("expected blink_led = true in output:\n%s", data)
	}
}
