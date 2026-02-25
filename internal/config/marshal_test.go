package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

func TestMarshal_RoundTrip(t *testing.T) {
	orig := Config{
		SDLogLevel:        LogLevelError,
		BluesLogLevel:     LogLevelWarn,
		SampleInterval:    time.Minute,
		SampleSensors:     []string{"temperature", "humidity"},
		SampleExtPin:      hal.NoPin,
		HeartbeatInterval: time.Hour,
		HeartbeatPayload:  HeartbeatPayloadFull,
		HeartbeatLedPin:   hal.Pin(16),
	}

	data, err := orig.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	got := testDefault()
	if err := ParseINI(data, &got); err != nil {
		t.Fatalf("ParseINI(MarshalINI()) error: %v\nINI:\n%s", err, data)
	}

	if got.SDLogLevel != LogLevelError {
		t.Errorf("SDLogLevel = %d, want LevelError", got.SDLogLevel)
	}
	if got.BluesLogLevel != LogLevelWarn {
		t.Errorf("BluesLogLevel = %d, want LevelWarn", got.BluesLogLevel)
	}
	if got.SampleInterval != time.Minute {
		t.Errorf("SampleInterval = %v, want 1m", got.SampleInterval)
	}
	if len(got.SampleSensors) != 2 {
		t.Errorf("SampleSensors = %v", got.SampleSensors)
	}
	if got.SampleExtPin != hal.NoPin {
		t.Errorf("SampleExtPin = %d, want NoPin", got.SampleExtPin)
	}
	if got.HeartbeatInterval != time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 1h", got.HeartbeatInterval)
	}
	if got.HeartbeatPayload != HeartbeatPayloadFull {
		t.Errorf("HeartbeatPayload = %d, want HeartbeatPayloadFull", got.HeartbeatPayload)
	}
	if got.HeartbeatLedPin != hal.Pin(16) {
		t.Errorf("HeartbeatLedPin = %d, want 16", got.HeartbeatLedPin)
	}
}

func TestMarshal_ZeroFieldsOmitted(t *testing.T) {
	cfg := Config{
		SDLogLevel:      LogLevelDebug,
		BluesLogLevel:   LogLevelDebug,
		SampleInterval:  5 * time.Second,
		SampleExtPin:    hal.NoPin,
		HeartbeatLedPin: hal.NoPin,
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
		SDLogLevel:    LogLevelDebug,
		BluesLogLevel: LogLevelDebug,
		SampleExtPin:  hal.Pin(7),
		SampleSensors: []string{"bucket"},
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
		SDLogLevel:      LogLevelDebug,
		BluesLogLevel:   LogLevelDebug,
		SampleInterval:  5 * time.Second,
		SampleExtPin:    hal.NoPin,
		HeartbeatLedPin: hal.NoPin,
		// HeartbeatInterval = 0 → disabled
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
			SDLogLevel:     LogLevelDebug,
			BluesLogLevel:  LogLevelDebug,
			SampleInterval: tt.d,
			SampleExtPin:   hal.NoPin,
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
		SDLogLevel:        LogLevelDebug,
		BluesLogLevel:     LogLevelDebug,
		SampleInterval:    time.Minute,
		SampleExtPin:      hal.NoPin,
		HeartbeatInterval: time.Hour,
		HeartbeatLedPin:   hal.Pin(16),
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
		SDLogLevel:        LogLevelError,
		BluesLogLevel:     LogLevelWarn,
		SampleInterval:    time.Minute,
		SampleSensors:     []string{"vbat", "temp"},
		SampleExtPin:      hal.Pin(7),
		HeartbeatInterval: time.Hour,
		HeartbeatPayload:  HeartbeatPayloadFull,
		HeartbeatLedPin:   hal.Pin(16),
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
