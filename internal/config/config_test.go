package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// testDefault returns a Default config with standard test board values.
func testDefault() Config {
	return Default(hal.Pin(13), []string{"vbat"}, nil, nil)
}

// --- Default ---

func TestDefault(t *testing.T) {
	cfg := testDefault()

	if cfg.LogLevelSD != LogLevelDebug {
		t.Errorf("LogLevelSD = %d, want LogLevelDebug", cfg.LogLevelSD)
	}
	if cfg.LogLevelBlues != LogLevelOff {
		t.Errorf("LogLevelBlues = %d, want LogLevelOff", cfg.LogLevelBlues)
	}

	if cfg.SampleInterval != 0 {
		t.Errorf("SampleInterval = %v, want 0s", cfg.SampleInterval)
	}
	if len(cfg.SampleSensors) != 1 || cfg.SampleSensors[0] != "vbat" {
		t.Errorf("SampleSensors = %v, want [vbat] (board-supplied)", cfg.SampleSensors)
	}
	if len(cfg.InterruptPins) != 0 {
		t.Errorf("InterruptPins = %v, want empty", cfg.InterruptPins)
	}

	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("HeartbeatInterval = %v, want 5s", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatPayload != HeartbeatPayloadNone {
		t.Errorf("HeartbeatPayload = %d, want HeartbeatPayloadNone", cfg.HeartbeatPayload)
	}
	if cfg.HeartbeatLedPin != hal.Pin(13) {
		t.Errorf("HeartbeatLedPin = %d, want 13 (board-supplied)", cfg.HeartbeatLedPin)
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
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.LogLevelSD != LogLevelError {
		t.Errorf("LogLevelSD = %d, want LogLevelError", cfg.LogLevelSD)
	}
	if cfg.LogLevelBlues != LogLevelWarn {
		t.Errorf("LogLevelBlues = %d, want LogLevelWarn", cfg.LogLevelBlues)
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
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.SampleInterval != 5*time.Minute {
		t.Errorf("SampleInterval = %v, want 5m", cfg.SampleInterval)
	}
}

func TestParseINI_SampleSensors(t *testing.T) {
	input := []byte("sample_sensors = vbat, temp\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if len(cfg.SampleSensors) != 2 || cfg.SampleSensors[0] != "vbat" || cfg.SampleSensors[1] != "temp" {
		t.Errorf("SampleSensors = %v, want [vbat temp]", cfg.SampleSensors)
	}
}

func TestParseINI_InterruptPins(t *testing.T) {
	// interrupt_pins alone (no interval) satisfies validation.
	input := []byte("interrupt_pins = 7\nsample_sensors = rain\n")
	cfg := Config{}
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if len(cfg.InterruptPins) != 1 || cfg.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("InterruptPins = %v, want [7]", cfg.InterruptPins)
	}
	if cfg.SampleInterval != 0 {
		t.Errorf("SampleInterval = %v, want 0", cfg.SampleInterval)
	}
}

func TestParseINI_InterruptPinsWithInterval(t *testing.T) {
	input := []byte("sample_interval = 5m\ninterrupt_pins = 7\nsample_sensors = rain\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.SampleInterval != 5*time.Minute {
		t.Errorf("SampleInterval = %v, want 5m", cfg.SampleInterval)
	}
	if len(cfg.InterruptPins) != 1 || cfg.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("InterruptPins = %v, want [7]", cfg.InterruptPins)
	}
}

func TestParseINI_SDChipSelectPins(t *testing.T) {
	input := []byte("sd_chip_select_pins = 11, 10\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if len(cfg.SDChipSelectPins) != 2 || cfg.SDChipSelectPins[0] != hal.Pin(11) || cfg.SDChipSelectPins[1] != hal.Pin(10) {
		t.Errorf("SDChipSelectPins = %v, want [11 10]", cfg.SDChipSelectPins)
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
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.HeartbeatInterval != 2*time.Minute {
		t.Errorf("HeartbeatInterval = %v, want 2m", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatPayload != HeartbeatPayloadMin {
		t.Errorf("HeartbeatPayload = %d, want HeartbeatPayloadMin", cfg.HeartbeatPayload)
	}
	if cfg.HeartbeatLedPin != hal.Pin(16) {
		t.Errorf("HeartbeatLedPin = %d, want 16", cfg.HeartbeatLedPin)
	}
}

func TestParseINI_HeartbeatDisabledByConfig(t *testing.T) {
	input := []byte("sample_interval = 9s\nheartbeat_interval = 0s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.HeartbeatInterval != 0 {
		t.Errorf("HeartbeatInterval = %v, want 0 (disabled)", cfg.HeartbeatInterval)
	}
}

func TestParseINI_HeartbeatLedPinNone(t *testing.T) {
	input := []byte("sample_interval = 9s\nheartbeat_led_pin = none\n")
	cfg := testDefault()
	cfg.HeartbeatLedPin = hal.Pin(13)
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.HeartbeatLedPin != hal.NoPin {
		t.Errorf("HeartbeatLedPin = %d, want NoPin", cfg.HeartbeatLedPin)
	}
}

// --- Validation ---

func TestParseINI_NoWakeSources(t *testing.T) {
	// No interval, no interrupt pins, no heartbeat — must be rejected.
	input := []byte("sd_log_level = warn\n")
	cfg := Config{} // zero-value: all intervals = 0, no pins
	err := ParseINI(input, &cfg)
	if err == nil {
		t.Fatal("expected error for no wake sources, got nil")
	}
	if !strings.Contains(err.Error(), "no wake sources") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseINI_HeartbeatOnlyWakeSource(t *testing.T) {
	// heartbeat_interval alone (no sample_interval or interrupt pins) satisfies validation.
	input := []byte("heartbeat_interval = 5m\n")
	cfg := Config{}
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.HeartbeatInterval != 5*time.Minute {
		t.Errorf("HeartbeatInterval = %v, want 5m", cfg.HeartbeatInterval)
	}
}

// --- ParseMap (env.get body interface) ---

func TestParseMap_AllKeys(t *testing.T) {
	body := map[string]interface{}{
		"sd_log_level":       "warn",
		"blues_log_level":    "error",
		"sample_interval":    "5m",
		"sample_sensors":     "vbat,temp",
		"interrupt_pins":     "7",
		"heartbeat_interval": "1h",
		"heartbeat_payload":  "full",
		"heartbeat_led_pin":  "16",
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}

	if cfg.LogLevelSD != LogLevelWarn {
		t.Errorf("LogLevelSD = %d, want LogLevelWarn", cfg.LogLevelSD)
	}
	if cfg.LogLevelBlues != LogLevelError {
		t.Errorf("LogLevelBlues = %d, want LogLevelError", cfg.LogLevelBlues)
	}
	if cfg.SampleInterval != 5*time.Minute {
		t.Errorf("SampleInterval = %v, want 5m", cfg.SampleInterval)
	}
	if len(cfg.InterruptPins) != 1 || cfg.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("InterruptPins = %v, want [7]", cfg.InterruptPins)
	}
	if cfg.HeartbeatInterval != time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 1h", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatPayload != HeartbeatPayloadFull {
		t.Errorf("HeartbeatPayload = %d, want HeartbeatPayloadFull", cfg.HeartbeatPayload)
	}
	if cfg.HeartbeatLedPin != hal.Pin(16) {
		t.Errorf("HeartbeatLedPin = %d, want 16", cfg.HeartbeatLedPin)
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
	if cfg.SampleInterval != 5*time.Minute {
		t.Errorf("SampleInterval = %v, want 5m", cfg.SampleInterval)
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
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.SampleInterval != 5*time.Minute {
		t.Errorf("Interval = %v, want 5m (inline comment should be stripped)", cfg.SampleInterval)
	}
}

func TestParseINI_PayloadInlineComment(t *testing.T) {
	input := []byte("heartbeat_interval = 1h\nheartbeat_payload = full # none | full | min\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.HeartbeatPayload != HeartbeatPayloadFull {
		t.Errorf("Payload = %d, want HeartbeatPayloadFull", cfg.HeartbeatPayload)
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
		t.Fatalf("ParseINI() error: %v", err)
	}
	if cfg.LogLevelSD != LogLevelWarn {
		t.Errorf("LogLevelSD = %d, want LogLevelWarn", cfg.LogLevelSD)
	}
	if cfg.SampleInterval != 3*time.Second {
		t.Errorf("Interval = %v, want 3s", cfg.SampleInterval)
	}
}

func TestParseINI_EmptyInput(t *testing.T) {
	cfg := testDefault()
	if err := ParseINI([]byte(""), &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}
	// Default settings preserved when nothing is parsed.
	if cfg.SampleInterval != 0 {
		t.Errorf("SampleInterval = %v, want 0", cfg.SampleInterval)
	}
	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("HeartbeatInterval = %v, want 5s", cfg.HeartbeatInterval)
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

func TestParseINI_InvalidPinList(t *testing.T) {
	input := []byte("interrupt_pins = abc\nsample_interval = 9s\n")
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err == nil {
		t.Fatal("expected error for invalid pin, got nil")
	}

	input2 := []byte("interrupt_pins = 256\nsample_interval = 9s\n")
	cfg2 := testDefault()
	if err := ParseINI(input2, &cfg2); err == nil {
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
		want  HeartbeatPayload
	}{
		{"none", HeartbeatPayloadNone},
		{"min", HeartbeatPayloadMin},
		{"full", HeartbeatPayloadFull},
	}
	for _, tt := range tests {
		input := []byte("heartbeat_interval = 1h\nheartbeat_payload = " + tt.value + "\nsample_interval = 9s\n")
		cfg := testDefault()
		if err := ParseINI(input, &cfg); err != nil {
			t.Fatalf("ParseINI(payload=%s) error: %v", tt.value, err)
		}
		if cfg.HeartbeatPayload != tt.want {
			t.Errorf("payload=%s: got %d, want %d", tt.value, cfg.HeartbeatPayload, tt.want)
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
interrupt_pins = 7

heartbeat_interval = 1h
heartbeat_payload = full
heartbeat_led_pin = 16
`)
	cfg := testDefault()
	if err := ParseINI(input, &cfg); err != nil {
		t.Fatalf("ParseINI() error: %v", err)
	}

	if cfg.LogLevelSD != LogLevelError {
		t.Errorf("LogLevelSD = %d, want LogLevelError", cfg.LogLevelSD)
	}
	if cfg.LogLevelBlues != LogLevelWarn {
		t.Errorf("LogLevelBlues = %d, want LogLevelWarn", cfg.LogLevelBlues)
	}
	if cfg.SampleInterval != time.Minute {
		t.Errorf("SampleInterval = %v, want 1m", cfg.SampleInterval)
	}
	if len(cfg.SampleSensors) != 2 {
		t.Errorf("SampleSensors = %v, want [temperature humidity]", cfg.SampleSensors)
	}
	if len(cfg.InterruptPins) != 1 || cfg.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("InterruptPins = %v, want [7]", cfg.InterruptPins)
	}

	if cfg.HeartbeatInterval != time.Hour {
		t.Errorf("HeartbeatInterval = %v, want 1h", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatPayload != HeartbeatPayloadFull {
		t.Errorf("HeartbeatPayload = %d, want HeartbeatPayloadFull", cfg.HeartbeatPayload)
	}
	if cfg.HeartbeatLedPin != hal.Pin(16) {
		t.Errorf("HeartbeatLedPin = %d, want 16", cfg.HeartbeatLedPin)
	}
}
