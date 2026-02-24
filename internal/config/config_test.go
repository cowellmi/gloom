package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// testDefault returns a Default config with the standard test board values.
func testDefault() Config {
	return Default(hal.Pin(13), []string{"vbat"})
}

// --- Default ---

func TestDefault(t *testing.T) {
	cfg := testDefault()

	if cfg.SD.LogLevel != log.LevelDebug {
		t.Errorf("SD.LogLevel = %d, want LevelDebug", cfg.SD.LogLevel)
	}
	if cfg.Blues.LogLevel != log.LevelInfo {
		t.Errorf("Blues.LogLevel = %d, want LevelInfo", cfg.Blues.LogLevel)
	}

	if cfg.Sample.Interval != 9*time.Second {
		t.Errorf("Sample.Interval = %v, want 9s", cfg.Sample.Interval)
	}
	if len(cfg.Sample.Sensors) != 1 || cfg.Sample.Sensors[0] != "vbat" {
		t.Errorf("Sample.Sensors = %v, want [vbat] (board-supplied)", cfg.Sample.Sensors)
	}
	if cfg.Sample.ExtPin != hal.NoPin {
		t.Errorf("Sample.ExtPin = %d, want NoPin", cfg.Sample.ExtPin)
	}

	if cfg.Heartbeat.Interval != 3*time.Second {
		t.Errorf("Heartbeat.Interval = %v, want 3s", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadNone {
		t.Errorf("Heartbeat.Payload = %d, want PayloadNone", cfg.Heartbeat.Payload)
	}
	if cfg.Heartbeat.LedPin != hal.Pin(13) {
		t.Errorf("Heartbeat.LedPin = %d, want 13 (board-supplied)", cfg.Heartbeat.LedPin)
	}
}

// --- SD / Blues log level keys ---

func TestParseINI_LogLevels(t *testing.T) {
	input := []byte(`
sd_log_level = error
blues_log_level = warn
sample_interval = 9s
`)
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.SD.LogLevel != log.LevelError {
		t.Errorf("SD.LogLevel = %d, want LevelError", cfg.SD.LogLevel)
	}
	if cfg.Blues.LogLevel != log.LevelWarn {
		t.Errorf("Blues.LogLevel = %d, want LevelWarn", cfg.Blues.LogLevel)
	}
}

func TestParseINI_LogLevelInvalid(t *testing.T) {
	input := []byte("sd_log_level = verbose\nsample_interval = 9s\n")
	cfg := testDefault()
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "unknown log level") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Sample keys ---

func TestParseINI_SampleInterval(t *testing.T) {
	input := []byte("sample_interval = 5m\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Sample.Interval = %v, want 5m", cfg.Sample.Interval)
	}
}

func TestParseINI_SampleSensors(t *testing.T) {
	input := []byte("sample_sensors = vbat, temp\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(cfg.Sample.Sensors) != 2 || cfg.Sample.Sensors[0] != "vbat" || cfg.Sample.Sensors[1] != "temp" {
		t.Errorf("Sample.Sensors = %v, want [vbat temp]", cfg.Sample.Sensors)
	}
}

func TestParseINI_SampleExtPin(t *testing.T) {
	// sample_ext_pin alone (no interval) satisfies validation.
	input := []byte("sample_ext_pin = 7\nsample_sensors = rain\n")
	cfg := Config{Sample: Sample{ExtPin: hal.NoPin}}
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}
	if cfg.Sample.Interval != 0 {
		t.Errorf("Sample.Interval = %v, want 0", cfg.Sample.Interval)
	}
}

func TestParseINI_SampleBothTriggers(t *testing.T) {
	input := []byte("sample_interval = 5m\nsample_ext_pin = 7\nsample_sensors = rain\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Sample.Interval = %v, want 5m", cfg.Sample.Interval)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}
}

// --- Heartbeat keys ---

func TestParseINI_HeartbeatKeys(t *testing.T) {
	input := []byte(`
sample_interval = 9s
heartbeat_interval = 2m
heartbeat_payload = min
heartbeat_led_pin = 16
`)
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Interval != 2*time.Minute {
		t.Errorf("Heartbeat.Interval = %v, want 2m", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadMin {
		t.Errorf("Heartbeat.Payload = %d, want PayloadMin", cfg.Heartbeat.Payload)
	}
	if cfg.Heartbeat.LedPin != hal.Pin(16) {
		t.Errorf("Heartbeat.LedPin = %d, want 16", cfg.Heartbeat.LedPin)
	}
}

func TestParseINI_HeartbeatDisabledByConfig(t *testing.T) {
	// Explicitly setting heartbeat_interval = 0s disables it even when Default has it enabled.
	input := []byte("sample_interval = 9s\nheartbeat_interval = 0s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Interval != 0 {
		t.Errorf("Heartbeat.Interval = %v, want 0 (disabled)", cfg.Heartbeat.Interval)
	}
}

func TestParseINI_HeartbeatLedPinNone(t *testing.T) {
	input := []byte("sample_interval = 9s\nheartbeat_led_pin = none\n")
	cfg := testDefault()
	cfg.Heartbeat.LedPin = hal.Pin(13)
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.LedPin != hal.NoPin {
		t.Errorf("Heartbeat.LedPin = %d, want NoPin", cfg.Heartbeat.LedPin)
	}
}

// --- ParseMap (env.get body interface) ---

func TestParseMap_AllKeys(t *testing.T) {
	body := map[string]interface{}{
		"sd_log_level":       "warn",
		"blues_log_level":    "error",
		"sample_interval":    "5m",
		"sample_sensors":     "vbat,temp",
		"sample_ext_pin":     "7",
		"heartbeat_interval": "1h",
		"heartbeat_payload":  "full",
		"heartbeat_led_pin":  "16",
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}

	if cfg.SD.LogLevel != log.LevelWarn {
		t.Errorf("SD.LogLevel = %d, want LevelWarn", cfg.SD.LogLevel)
	}
	if cfg.Blues.LogLevel != log.LevelError {
		t.Errorf("Blues.LogLevel = %d, want LevelError", cfg.Blues.LogLevel)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Sample.Interval = %v, want 5m", cfg.Sample.Interval)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}
	if cfg.Heartbeat.Interval != time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want 1h", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadFull {
		t.Errorf("Heartbeat.Payload = %d, want PayloadFull", cfg.Heartbeat.Payload)
	}
	if cfg.Heartbeat.LedPin != hal.Pin(16) {
		t.Errorf("Heartbeat.LedPin = %d, want 16", cfg.Heartbeat.LedPin)
	}
}

func TestParseMap_NotehubInternalKeysSkipped(t *testing.T) {
	body := map[string]interface{}{
		"_tri_mins":       "60",
		"sample_interval": "5m",
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Sample.Interval = %v, want 5m", cfg.Sample.Interval)
	}
}

func TestParseMap_UnknownKey(t *testing.T) {
	body := map[string]interface{}{"bad_key": "x"}
	cfg := testDefault()
	err := ParseMap(&cfg, body)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Inline comments ---

func TestParseINI_InlineComments(t *testing.T) {
	input := []byte("sample_interval = 5m # every five minutes\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want 5m (inline comment should be stripped)", cfg.Sample.Interval)
	}
}

func TestParseINI_PayloadInlineComment(t *testing.T) {
	input := []byte("heartbeat_interval = 1h\nheartbeat_payload = full # none | full | min\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Payload != PayloadFull {
		t.Errorf("Payload = %d, want PayloadFull", cfg.Heartbeat.Payload)
	}
}

// --- Comments and blanks ---

func TestParseINI_CommentsAndBlanks(t *testing.T) {
	input := []byte(`
# This is a comment

# Another comment
sd_log_level = warn

sample_interval = 3s
sample_sensors = vbat
`)
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.SD.LogLevel != log.LevelWarn {
		t.Errorf("SD.LogLevel = %d, want LevelWarn", cfg.SD.LogLevel)
	}
	if cfg.Sample.Interval != 3*time.Second {
		t.Errorf("Interval = %v, want 3s", cfg.Sample.Interval)
	}
}

func TestParseINI_EmptyInput(t *testing.T) {
	cfg := testDefault()
	if err := ParseINI([]byte(""), &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// Default settings preserved when nothing is parsed.
	if cfg.Sample.Interval != 9*time.Second {
		t.Errorf("Sample.Interval = %v, want 9s", cfg.Sample.Interval)
	}
	if cfg.Heartbeat.Interval != 3*time.Second {
		t.Errorf("Heartbeat.Interval = %v, want 3s", cfg.Heartbeat.Interval)
	}
}

// --- Validation ---

func TestParseINI_ValidationMissingTrigger(t *testing.T) {
	// No interval, no ext_pin → validation fails.
	input := []byte("sample_sensors = vbat\n")
	cfg := Config{Sample: Sample{ExtPin: hal.NoPin}}
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for sample without interval or ext_pin, got nil")
	}
	if !strings.Contains(err.Error(), "must have sample_interval or sample_ext_pin") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Error handling ---

func TestParseINI_InvalidDuration(t *testing.T) {
	input := []byte("sample_interval = bad\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestParseINI_NegativeDuration(t *testing.T) {
	input := []byte("sample_interval = -5s\n")
	cfg := testDefault()
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for negative duration, got nil")
	}
	if !strings.Contains(err.Error(), "negative duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseINI_InvalidPin(t *testing.T) {
	input := []byte("sample_ext_pin = abc\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err == nil {
		t.Fatal("expected error for invalid pin, got nil")
	}
}

func TestParseINI_PinOverflow(t *testing.T) {
	input := []byte("sample_ext_pin = 256\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err == nil {
		t.Fatal("expected error for pin overflow, got nil")
	}
}

func TestParseINI_UnknownPayload(t *testing.T) {
	input := []byte("heartbeat_payload = mega\nsample_interval = 9s\n")
	cfg := testDefault()
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown payload, got nil")
	}
	if !strings.Contains(err.Error(), "unknown payload") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseINI_UnknownKey(t *testing.T) {
	input := []byte("bad_key = x\nsample_interval = 9s\n")
	cfg := testDefault()
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseINI_MultipleErrors(t *testing.T) {
	input := []byte("bad_key1 = x\nbad_key2 = y\n")
	cfg := testDefault()
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected errors, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad_key1") {
		t.Errorf("error should mention bad_key1, got: %s", msg)
	}
	if !strings.Contains(msg, "bad_key2") {
		t.Errorf("error should mention bad_key2, got: %s", msg)
	}
}

// --- Payload parsing ---

func TestParseINI_PayloadVariants(t *testing.T) {
	tests := []struct {
		value string
		want  Payload
	}{
		{"none", PayloadNone},
		{"min", PayloadMin},
		{"full", PayloadFull},
	}
	for _, tt := range tests {
		input := []byte("heartbeat_interval = 1h\nheartbeat_payload = " + tt.value + "\nsample_interval = 9s\n")
		cfg := testDefault()
		if err := ParseINI(input, &cfg); err != nil {
			t.Fatalf("Parse(payload=%s) error: %v", tt.value, err)
		}
		if cfg.Heartbeat.Payload != tt.want {
			t.Errorf("payload=%s: got %d, want %d", tt.value, cfg.Heartbeat.Payload, tt.want)
		}
	}
}

// --- Full example ---

func TestParseINI_FullExample(t *testing.T) {
	input := []byte(`
# Full flat config example
sd_log_level = error
blues_log_level = warn

sample_interval = 1m
sample_sensors = temperature, humidity
sample_ext_pin = 7

heartbeat_interval = 1h
heartbeat_payload = full
heartbeat_led_pin = 16
`)
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.SD.LogLevel != log.LevelError {
		t.Errorf("SD.LogLevel = %d, want LevelError", cfg.SD.LogLevel)
	}
	if cfg.Blues.LogLevel != log.LevelWarn {
		t.Errorf("Blues.LogLevel = %d, want LevelWarn", cfg.Blues.LogLevel)
	}

	if cfg.Sample.Interval != time.Minute {
		t.Errorf("Sample.Interval = %v, want 1m", cfg.Sample.Interval)
	}
	if len(cfg.Sample.Sensors) != 2 {
		t.Errorf("Sample.Sensors = %v, want [temperature humidity]", cfg.Sample.Sensors)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}

	if cfg.Heartbeat.Interval != time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want 1h", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadFull {
		t.Errorf("Heartbeat.Payload = %d, want PayloadFull", cfg.Heartbeat.Payload)
	}
	if cfg.Heartbeat.LedPin != hal.Pin(16) {
		t.Errorf("Heartbeat.LedPin = %d, want 16", cfg.Heartbeat.LedPin)
	}
}
