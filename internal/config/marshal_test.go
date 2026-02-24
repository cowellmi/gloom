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
		SD:    SD{LogLevel: log.LevelError},
		Blues: Blues{LogLevel: log.LevelWarn},
		Sample: Sample{
			Interval: time.Minute,
			Sensors:  []string{"temperature", "humidity"},
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			Interval: time.Hour,
			Payload:  PayloadFull,
			LedPin:   hal.Pin(16),
		},
	}

	data, err := orig.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	// Start from Default so NoPin sentinels are correctly seeded for
	// fields absent from the marshaled output.
	got := testDefault()
	if err := ParseINI(data, &got); err != nil {
		t.Fatalf("Parse(MarshalINI()) error: %v\nINI:\n%s", err, data)
	}

	if got.SD.LogLevel != log.LevelError {
		t.Errorf("SD.LogLevel = %d, want LevelError", got.SD.LogLevel)
	}
	if got.Blues.LogLevel != log.LevelWarn {
		t.Errorf("Blues.LogLevel = %d, want LevelWarn", got.Blues.LogLevel)
	}
	if got.Sample.Interval != time.Minute {
		t.Errorf("Sample.Interval = %v, want 1m", got.Sample.Interval)
	}
	if len(got.Sample.Sensors) != 2 {
		t.Errorf("Sample.Sensors = %v", got.Sample.Sensors)
	}
	if got.Sample.ExtPin != hal.NoPin {
		t.Errorf("Sample.ExtPin = %d, want NoPin", got.Sample.ExtPin)
	}
	if got.Heartbeat.Interval != time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want 1h", got.Heartbeat.Interval)
	}
	if got.Heartbeat.Payload != PayloadFull {
		t.Errorf("Heartbeat.Payload = %d, want PayloadFull", got.Heartbeat.Payload)
	}
	if got.Heartbeat.LedPin != hal.Pin(16) {
		t.Errorf("Heartbeat.LedPin = %d, want 16", got.Heartbeat.LedPin)
	}
}

func TestMarshal_ZeroFieldsOmitted(t *testing.T) {
	cfg := Config{
		SD:    SD{LogLevel: log.LevelDebug},
		Blues: Blues{LogLevel: log.LevelDebug},
		Sample: Sample{
			Interval: 5 * time.Second,
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			LedPin: hal.NoPin,
		},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	s := string(data)
	for _, absent := range []string{
		"sample_sensors", "sample_ext_pin",
		"heartbeat_interval", "heartbeat_payload", "heartbeat_led_pin",
	} {
		if strings.Contains(s, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, s)
		}
	}

	if !strings.Contains(s, "sample_interval = 5s") {
		t.Errorf("output should contain 'sample_interval = 5s':\n%s", s)
	}
}

func TestMarshal_ExtPin(t *testing.T) {
	cfg := Config{
		SD:    SD{LogLevel: log.LevelDebug},
		Blues: Blues{LogLevel: log.LevelDebug},
		Sample: Sample{
			ExtPin:  hal.Pin(7),
			Sensors: []string{"bucket"},
		},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	if !strings.Contains(string(data), "sample_ext_pin = 7") {
		t.Errorf("expected sample_ext_pin = 7 in output:\n%s", data)
	}
}

func TestMarshal_HeartbeatDisabled(t *testing.T) {
	cfg := Config{
		SD:    SD{LogLevel: log.LevelDebug},
		Blues: Blues{LogLevel: log.LevelDebug},
		Sample: Sample{
			Interval: 5 * time.Second,
			ExtPin:   hal.NoPin,
		},
		// Heartbeat.Interval = 0 → disabled; LedPin explicit NoPin
		Heartbeat: Heartbeat{LedPin: hal.NoPin},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
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
			SD:    SD{LogLevel: log.LevelDebug},
			Blues: Blues{LogLevel: log.LevelDebug},
			Sample: Sample{
				Interval: tt.d,
				ExtPin:   hal.NoPin,
			},
		}
		data, err := cfg.MarshalINI()
		if err != nil {
			t.Fatalf("MarshalINI() error for %v: %v", tt.d, err)
		}
		if !strings.Contains(string(data), "sample_interval = "+tt.want) {
			t.Errorf("duration %v: want %q in output:\n%s", tt.d, tt.want, data)
		}
	}
}

func TestMarshal_LedPin(t *testing.T) {
	cfg := Config{
		SD:    SD{LogLevel: log.LevelDebug},
		Blues: Blues{LogLevel: log.LevelDebug},
		Sample: Sample{
			Interval: time.Minute,
			ExtPin:   hal.NoPin,
		},
		Heartbeat: Heartbeat{
			Interval: time.Hour,
			LedPin:   hal.Pin(16),
		},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	if !strings.Contains(string(data), "heartbeat_led_pin = 16") {
		t.Errorf("expected heartbeat_led_pin = 16 in output:\n%s", data)
	}
}

func TestMarshalMap(t *testing.T) {
	cfg := Config{
		SD:    SD{LogLevel: log.LevelError},
		Blues: Blues{LogLevel: log.LevelWarn},
		Sample: Sample{
			Interval: time.Minute,
			Sensors:  []string{"vbat", "temp"},
			ExtPin:   hal.Pin(7),
		},
		Heartbeat: Heartbeat{
			Interval: time.Hour,
			Payload:  PayloadFull,
			LedPin:   hal.Pin(16),
		},
	}

	m := cfg.MarshalMap()

	if m["sd_log_level"] != "error" {
		t.Errorf("sd_log_level = %v, want error", m["sd_log_level"])
	}
	if m["blues_log_level"] != "warn" {
		t.Errorf("blues_log_level = %v, want warn", m["blues_log_level"])
	}
	if m["sample_interval"] != "1m" {
		t.Errorf("sample_interval = %v, want 1m", m["sample_interval"])
	}
	if m["sample_sensors"] != "vbat, temp" {
		t.Errorf("sample_sensors = %v, want vbat, temp", m["sample_sensors"])
	}
	if m["sample_ext_pin"] != "7" {
		t.Errorf("sample_ext_pin = %v, want 7", m["sample_ext_pin"])
	}
	if m["heartbeat_interval"] != "1h" {
		t.Errorf("heartbeat_interval = %v, want 1h", m["heartbeat_interval"])
	}
	if m["heartbeat_payload"] != "full" {
		t.Errorf("heartbeat_payload = %v, want full", m["heartbeat_payload"])
	}
	if m["heartbeat_led_pin"] != "16" {
		t.Errorf("heartbeat_led_pin = %v, want 16", m["heartbeat_led_pin"])
	}
}

func TestMarshalMap_NoPin(t *testing.T) {
	cfg := Default(hal.NoPin, nil)

	m := cfg.MarshalMap()

	if m["sample_ext_pin"] != "none" {
		t.Errorf("sample_ext_pin = %v, want none", m["sample_ext_pin"])
	}
	if m["heartbeat_led_pin"] != "none" {
		t.Errorf("heartbeat_led_pin = %v, want none", m["heartbeat_led_pin"])
	}
}
