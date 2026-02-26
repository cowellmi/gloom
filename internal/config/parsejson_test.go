package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// --- ParseJSON ---

func TestParseJSON_BasicRoundTrip(t *testing.T) {
	orig := Default(hal.Pin(13), hal.Pin(5), []string{"vbat"}, []hal.Pin{11})
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	// Seed from Default() so merge semantics on sinks have a base to work from.
	got := Default(hal.Pin(13), hal.Pin(5), []string{"vbat"}, []hal.Pin{11})
	if err := ParseJSON(data, &got); err != nil {
		t.Fatalf("ParseJSON(MarshalJSON()) error: %v\nJSON:\n%s", err, data)
	}

	if got.LEDPin != hal.Pin(13) {
		t.Errorf("LEDPin = %d, want 13", got.LEDPin)
	}
	if got.RTCIntPin != hal.Pin(5) {
		t.Errorf("RTCIntPin = %d, want 5", got.RTCIntPin)
	}
	if len(got.SDChipSelectPins) != 1 || got.SDChipSelectPins[0] != hal.Pin(11) {
		t.Errorf("SDChipSelectPins = %v, want [11]", got.SDChipSelectPins)
	}
	if got.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues log level = %v, want off", got.Sinks["blues"].LogLevel)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("Groups len = %d, want 1", len(got.Groups))
	}
	g := got.Groups["sample"]
	if g.Interval != 5*time.Second {
		t.Errorf("sample.Interval = %v, want 5s", g.Interval)
	}
	if !g.PulseLED {
		t.Error("sample.PulseLED = false, want true")
	}
}

func TestParseJSON_DefaultRoundTrip(t *testing.T) {
	orig := Default(hal.Pin(13), hal.Pin(5), []string{"vbat"}, []hal.Pin{11})
	data, err := orig.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	got := Default(hal.Pin(13), hal.Pin(5), []string{"vbat"}, []hal.Pin{11})
	if err := ParseJSON(data, &got); err != nil {
		t.Fatalf("ParseJSON error: %v\nJSON:\n%s", err, data)
	}

	if got.Sinks["serial"].LogLevel != LogLevelDebug {
		t.Errorf("serial = %v, want debug", got.Sinks["serial"].LogLevel)
	}
	if got.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues = %v, want off", got.Sinks["blues"].LogLevel)
	}
	if got.Sinks["sd"].LogLevel != LogLevelDebug {
		t.Errorf("sd = %v, want debug", got.Sinks["sd"].LogLevel)
	}
}

func TestParseJSON_LEDPin(t *testing.T) {
	input := []byte(`{"led_pin": 16, "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if cfg.LEDPin != hal.Pin(16) {
		t.Errorf("LEDPin = %d, want 16", cfg.LEDPin)
	}
}

func TestParseJSON_RTCIntPin(t *testing.T) {
	input := []byte(`{"rtc_int_pin": 7, "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if cfg.RTCIntPin != hal.Pin(7) {
		t.Errorf("RTCIntPin = %d, want 7", cfg.RTCIntPin)
	}
}

func TestParseJSON_SDChipSelectPins(t *testing.T) {
	input := []byte(`{"sd_chip_select_pins": [11, 10], "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if len(cfg.SDChipSelectPins) != 2 || cfg.SDChipSelectPins[0] != hal.Pin(11) || cfg.SDChipSelectPins[1] != hal.Pin(10) {
		t.Errorf("SDChipSelectPins = %v, want [11 10]", cfg.SDChipSelectPins)
	}
}

func TestParseJSON_Sinks(t *testing.T) {
	input := []byte(`{
  "sinks": {
    "serial": {"log_level": "warn"},
    "blues": {"log_level": "error"},
    "sd": {"log_level": "info"}
  },
  "groups": {"s": {"interval": "5s"}}
}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if cfg.Sinks["serial"].LogLevel != LogLevelWarn {
		t.Errorf("serial = %v, want warn", cfg.Sinks["serial"].LogLevel)
	}
	if cfg.Sinks["blues"].LogLevel != LogLevelError {
		t.Errorf("blues = %v, want error", cfg.Sinks["blues"].LogLevel)
	}
	if cfg.Sinks["sd"].LogLevel != LogLevelInfo {
		t.Errorf("sd = %v, want info", cfg.Sinks["sd"].LogLevel)
	}
}

func TestParseJSON_SinksMerge(t *testing.T) {
	// Only "serial" updated; other sinks keep defaults.
	input := []byte(`{
  "sinks": {"serial": {"log_level": "warn"}},
  "groups": {"s": {"interval": "5s"}}
}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if cfg.Sinks["serial"].LogLevel != LogLevelWarn {
		t.Errorf("serial = %v, want warn", cfg.Sinks["serial"].LogLevel)
	}
	if cfg.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues should remain off, got %v", cfg.Sinks["blues"].LogLevel)
	}
}

func TestParseJSON_SinksLogLevelOff(t *testing.T) {
	input := []byte(`{"sinks": {"blues": {"log_level": "off"}}, "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if cfg.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues = %v, want off", cfg.Sinks["blues"].LogLevel)
	}
}

func TestParseJSON_Groups(t *testing.T) {
	input := []byte(`{
  "groups": {
    "sample": {"interval": "5m", "sensors": ["vbat", "temp"], "pulse_led": true},
    "rain": {"interrupt_pins": [7], "sensors": ["bucket"]}
  }
}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if len(cfg.Groups) != 2 {
		t.Fatalf("Groups len = %d, want 2", len(cfg.Groups))
	}

	sample := cfg.Groups["sample"]
	if sample.Interval != 5*time.Minute {
		t.Errorf("sample.Interval = %v, want 5m", sample.Interval)
	}
	if len(sample.Sensors) != 2 || sample.Sensors[0] != "vbat" || sample.Sensors[1] != "temp" {
		t.Errorf("sample.Sensors = %v, want [vbat temp]", sample.Sensors)
	}
	if !sample.PulseLED {
		t.Error("sample.PulseLED = false, want true")
	}

	rain := cfg.Groups["rain"]
	if len(rain.InterruptPins) != 1 || rain.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("rain.InterruptPins = %v, want [7]", rain.InterruptPins)
	}
	if len(rain.Sensors) != 1 || rain.Sensors[0] != "bucket" {
		t.Errorf("rain.Sensors = %v, want [bucket]", rain.Sensors)
	}
}

func TestParseJSON_GroupsClearDefaults(t *testing.T) {
	input := []byte(`{"groups": {"custom": {"interval": "1h"}}}`)
	cfg := testDefault() // has default "sample" group
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups len = %d, want 1 (default cleared)", len(cfg.Groups))
	}
	if _, ok := cfg.Groups["custom"]; !ok {
		t.Error("Groups missing 'custom'")
	}
	if _, ok := cfg.Groups["sample"]; ok {
		t.Error("default 'sample' group should have been cleared")
	}
}

func TestParseJSON_NoGroupsInFile_KeepsDefaults(t *testing.T) {
	// JSON with no "groups" key keeps the default groups.
	input := []byte(`{"led_pin": 16}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups len = %d, want 1 (default preserved)", len(cfg.Groups))
	}
}

func TestParseJSON_UnknownTopLevelKey(t *testing.T) {
	input := []byte(`{"bad_key": "x", "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	err := ParseJSON(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseJSON_InvalidDuration(t *testing.T) {
	input := []byte(`{"groups": {"s": {"interval": "bad"}}}`)
	cfg := testDefault()
	if err := ParseJSON(input, &cfg); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestParseJSON_NegativeDuration(t *testing.T) {
	input := []byte(`{"groups": {"s": {"interval": "-5s"}}}`)
	cfg := testDefault()
	err := ParseJSON(input, &cfg)
	if err == nil {
		t.Fatal("expected error for negative duration, got nil")
	}
	if !strings.Contains(err.Error(), "negative duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseJSON_InterruptPinsAloneIsValidWakeSource(t *testing.T) {
	input := []byte(`{"groups": {"rain": {"interrupt_pins": [7], "sensors": ["bucket"]}}}`)
	cfg := Config{Sinks: testDefault().Sinks}
	if err := ParseJSON(input, &cfg); err != nil {
		t.Fatalf("ParseJSON() error: %v", err)
	}
}

func TestParseJSON_NoWakeSources(t *testing.T) {
	input := []byte(`{"groups": {"s": {"sensors": ["vbat"]}}}`)
	cfg := Config{Sinks: testDefault().Sinks}
	err := ParseJSON(input, &cfg)
	if err == nil {
		t.Fatal("expected error for no wake sources, got nil")
	}
	if !strings.Contains(err.Error(), "no wake source") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseJSON_UnknownGroupKey(t *testing.T) {
	input := []byte(`{"groups": {"s": {"interval": "5s", "bad_field": "x"}}}`)
	cfg := testDefault()
	err := ParseJSON(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown group key, got nil")
	}
}

func TestParseJSON_InvalidSinkLogLevel(t *testing.T) {
	input := []byte(`{"sinks": {"serial": {"log_level": "verbose"}}, "groups": {"s": {"interval": "5s"}}}`)
	cfg := testDefault()
	err := ParseJSON(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "unknown log level") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	cfg := testDefault()
	if err := ParseJSON([]byte("not json"), &cfg); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseJSON_EmptyObject(t *testing.T) {
	// Empty JSON object: no groups → Validate fails.
	cfg := Config{}
	err := ParseJSON([]byte(`{}`), &cfg)
	if err == nil {
		t.Fatal("expected error for no groups, got nil")
	}
	if !strings.Contains(err.Error(), "no groups defined") {
		t.Errorf("unexpected error: %v", err)
	}
}
