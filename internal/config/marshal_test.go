package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

func TestMarshal_RoundTrip(t *testing.T) {
	orig := Config{
		LogLevelSD:        LogLevelError,
		LogLevelBlues:     LogLevelWarn,
		SampleInterval:    time.Minute,
		SampleSensors:     []string{"temperature", "humidity"},
		InterruptPins:     []hal.Pin{7},
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

	if got.LogLevelSD != LogLevelError {
		t.Errorf("LogLevelSD = %d, want LevelError", got.LogLevelSD)
	}
	if got.LogLevelBlues != LogLevelWarn {
		t.Errorf("LogLevelBlues = %d, want LevelWarn", got.LogLevelBlues)
	}
	if got.SampleInterval != time.Minute {
		t.Errorf("SampleInterval = %v, want 1m", got.SampleInterval)
	}
	if len(got.SampleSensors) != 2 {
		t.Errorf("SampleSensors = %v", got.SampleSensors)
	}
	if len(got.InterruptPins) != 1 || got.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("InterruptPins = %v, want [7]", got.InterruptPins)
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
		LogLevelSD:      LogLevelDebug,
		LogLevelBlues:   LogLevelDebug,
		SampleInterval:  5 * time.Second,
		HeartbeatLedPin: hal.NoPin,
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	s := string(data)
	for _, absent := range []string{
		"sample_sensors", "interrupt_pins", "sd_chip_select_pins",
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

func TestMarshal_InterruptPins(t *testing.T) {
	cfg := Config{
		LogLevelSD:    LogLevelDebug,
		LogLevelBlues: LogLevelDebug,
		InterruptPins: []hal.Pin{7},
		SampleSensors: []string{"bucket"},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	if !strings.Contains(string(data), "interrupt_pins = 7") {
		t.Errorf("expected interrupt_pins = 7 in output:\n%s", data)
	}
}

func TestMarshal_SDChipSelectPins(t *testing.T) {
	cfg := Config{
		LogLevelSD:       LogLevelDebug,
		LogLevelBlues:    LogLevelDebug,
		SampleInterval:   5 * time.Second,
		SDChipSelectPins: []hal.Pin{11, 10},
	}

	data, err := cfg.MarshalINI()
	if err != nil {
		t.Fatalf("MarshalINI() error: %v", err)
	}

	if !strings.Contains(string(data), "sd_chip_select_pins = 11, 10") {
		t.Errorf("expected sd_chip_select_pins = 11, 10 in output:\n%s", data)
	}
}

func TestMarshal_HeartbeatDisabled(t *testing.T) {
	cfg := Config{
		LogLevelSD:      LogLevelDebug,
		LogLevelBlues:   LogLevelDebug,
		SampleInterval:  5 * time.Second,
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
			LogLevelSD:     LogLevelDebug,
			LogLevelBlues:  LogLevelDebug,
			SampleInterval: tt.d,
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
		LogLevelSD:        LogLevelDebug,
		LogLevelBlues:     LogLevelDebug,
		SampleInterval:    time.Minute,
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
		LogLevelSD:        LogLevelError,
		LogLevelBlues:     LogLevelWarn,
		SampleInterval:    time.Minute,
		SampleSensors:     []string{"vbat", "temp"},
		InterruptPins:     []hal.Pin{7},
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
	if m["interrupt_pins"] != "7" {
		t.Errorf("interrupt_pins = %v, want 7", m["interrupt_pins"])
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

func TestMarshalMap_EmptySlicesOmitted(t *testing.T) {
	cfg := Default(hal.NoPin, nil, nil, nil)

	m := cfg.MarshalMap()

	if _, ok := m["interrupt_pins"]; ok {
		t.Errorf("interrupt_pins should be absent from map when empty, got %v", m["interrupt_pins"])
	}
	if _, ok := m["sd_chip_select_pins"]; ok {
		t.Errorf("sd_chip_select_pins should be absent from map when empty, got %v", m["sd_chip_select_pins"])
	}
	if m["heartbeat_led_pin"] != "none" {
		t.Errorf("heartbeat_led_pin = %v, want none", m["heartbeat_led_pin"])
	}
}
