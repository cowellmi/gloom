package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

// --- Default ---

func TestDefault(t *testing.T) {
	cfg := Default()

	if len(cfg.Device.LogSinks) != 0 {
		t.Errorf("LogSinks = %v, want empty", cfg.Device.LogSinks)
	}
	if len(cfg.Device.DataSinks) != 0 {
		t.Errorf("DataSinks = %v, want empty", cfg.Device.DataSinks)
	}
	if cfg.Device.LedPin != hal.NoPin {
		t.Errorf("LedPin = %d, want NoPin", cfg.Device.LedPin)
	}

	if cfg.Sample.Interval != 9*time.Second {
		t.Errorf("Sample.Interval = %v, want 9s", cfg.Sample.Interval)
	}
	if len(cfg.Sample.Sensors) != 1 || cfg.Sample.Sensors[0] != "vbat" {
		t.Errorf("Sample.Sensors = %v, want [vbat]", cfg.Sample.Sensors)
	}
	if cfg.Sample.ExtPin != hal.NoPin {
		t.Errorf("Sample.ExtPin = %d, want NoPin", cfg.Sample.ExtPin)
	}

	if cfg.Heartbeat.Interval != 3*time.Second {
		t.Errorf("Heartbeat.Interval = %v, want 3s", cfg.Heartbeat.Interval)
	}
	if !cfg.Heartbeat.BlinkLED {
		t.Error("Heartbeat.BlinkLED = false, want true")
	}
}

// --- Device keys ---

func TestParse_DeviceKeys(t *testing.T) {
	input := []byte(`
log_sinks = sd:error
data_sinks = sd
led_pin = 13
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Device.LogSinks) != 1 {
		t.Fatalf("LogSinks = %v, want 1", cfg.Device.LogSinks)
	}
	if cfg.Device.LogSinks[0].Name != "sd" || cfg.Device.LogSinks[0].Level != log.LevelError {
		t.Errorf("LogSinks[0] = %+v, want sd:error", cfg.Device.LogSinks[0])
	}
	if len(cfg.Device.DataSinks) != 1 || cfg.Device.DataSinks[0] != "sd" {
		t.Errorf("DataSinks = %v, want [sd]", cfg.Device.DataSinks)
	}
	if cfg.Device.LedPin != hal.Pin(13) {
		t.Errorf("LedPin = %d, want 13", cfg.Device.LedPin)
	}
}

func TestParse_LogSinksDefaultLevel(t *testing.T) {
	input := []byte("log_sinks = sd\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	for _, s := range cfg.Device.LogSinks {
		if s.Level != log.LevelDebug {
			t.Errorf("LogSink %q level = %d, want LevelDebug", s.Name, s.Level)
		}
	}
}

func TestParse_LogSinksInvalidLevel(t *testing.T) {
	input := []byte("log_sinks = sd:verbose\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "unknown log level") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Sample keys ---

func TestParse_SampleInterval(t *testing.T) {
	input := []byte("interval = 5m\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Sample.Interval = %v, want 5m", cfg.Sample.Interval)
	}
}

func TestParse_SampleSensors(t *testing.T) {
	input := []byte("sensors = vbat, temp\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(cfg.Sample.Sensors) != 2 || cfg.Sample.Sensors[0] != "vbat" || cfg.Sample.Sensors[1] != "temp" {
		t.Errorf("Sample.Sensors = %v, want [vbat temp]", cfg.Sample.Sensors)
	}
}

func TestParse_SampleExtPin(t *testing.T) {
	// ext_pin alone (no interval) satisfies validation.
	input := []byte("ext_pin = 7\nsensors = rain\n")
	cfg := Config{Sample: Sample{ExtPin: hal.NoPin}}
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}
	if cfg.Sample.Interval != 0 {
		t.Errorf("Sample.Interval = %v, want 0", cfg.Sample.Interval)
	}
}

func TestParse_SampleBothTriggers(t *testing.T) {
	input := []byte("interval = 5m\next_pin = 7\nsensors = rain\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
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

func TestParse_HeartbeatKeys(t *testing.T) {
	input := []byte(`
heartbeat = 2m
payload = min
blink_led = true
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Interval != 2*time.Minute {
		t.Errorf("Heartbeat.Interval = %v, want 2m", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadMin {
		t.Errorf("Heartbeat.Payload = %d, want PayloadMin", cfg.Heartbeat.Payload)
	}
	if !cfg.Heartbeat.BlinkLED {
		t.Error("Heartbeat.BlinkLED = false, want true")
	}
}

func TestParse_HeartbeatDisabledByConfig(t *testing.T) {
	// Explicitly setting heartbeat = 0s disables it even when Default has it enabled.
	input := []byte("heartbeat = 0s\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Interval != 0 {
		t.Errorf("Heartbeat.Interval = %v, want 0 (disabled)", cfg.Heartbeat.Interval)
	}
}

func TestParse_BlinkLED(t *testing.T) {
	for _, val := range []string{"true", "True", "TRUE", "yes", "1"} {
		input := []byte("blink_led = " + val)
		cfg := Default()
		if err := Parse(input, &cfg); err != nil {
			t.Fatalf("Parse(blink_led=%s) error: %v", val, err)
		}
		if !cfg.Heartbeat.BlinkLED {
			t.Errorf("blink_led = %s: want true", val)
		}
	}

	input := []byte("blink_led = false")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse(blink_led=false) error: %v", err)
	}
	if cfg.Heartbeat.BlinkLED {
		t.Error("blink_led = false: want false")
	}
}

// --- ParseMap (env.get body interface) ---

func TestParseMap_AllKeys(t *testing.T) {
	body := map[string]interface{}{
		"log_sinks":  "sd:warn",
		"data_sinks": "sd",
		"led_pin":    "13",
		"interval":   "5m",
		"sensors":    "vbat,temp",
		"ext_pin":    "7",
		"heartbeat":  "1h",
		"payload":    "full",
		"blink_led":  true, // native bool from Notecard JSON decoder
	}
	cfg := Default()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}

	if cfg.Device.LedPin != hal.Pin(13) {
		t.Errorf("LedPin = %d, want 13", cfg.Device.LedPin)
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
	if !cfg.Heartbeat.BlinkLED {
		t.Error("Heartbeat.BlinkLED = false, want true")
	}
}
func TestParseMap_UnknownKey(t *testing.T) {
	body := map[string]interface{}{"bad_key": "x"}
	cfg := Default()
	err := ParseMap(&cfg, body)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Inline comments ---

func TestParse_InlineComments(t *testing.T) {
	input := []byte("interval = 5m # every five minutes\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Sample.Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want 5m (inline comment should be stripped)", cfg.Sample.Interval)
	}
}

func TestParse_PayloadInlineComment(t *testing.T) {
	input := []byte("heartbeat = 1h\npayload = full # none | full | min\n")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.Heartbeat.Payload != PayloadFull {
		t.Errorf("Payload = %d, want PayloadFull", cfg.Heartbeat.Payload)
	}
}

// --- Comments and blanks ---

func TestParse_CommentsAndBlanks(t *testing.T) {
	input := []byte(`
# This is a comment

# Another comment
log_sinks = sd

interval = 3s
sensors = vbat
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Device.LogSinks) != 1 || cfg.Device.LogSinks[0].Name != "sd" {
		t.Errorf("LogSinks = %v, want [sd]", cfg.Device.LogSinks)
	}
	if cfg.Sample.Interval != 3*time.Second {
		t.Errorf("Interval = %v, want 3s", cfg.Sample.Interval)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	cfg := Default()
	if err := Parse([]byte(""), &cfg); err != nil {
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

func TestParse_ValidationMissingTrigger(t *testing.T) {
	// No interval, no ext_pin → validation fails.
	input := []byte("sensors = vbat\n")
	cfg := Config{Sample: Sample{ExtPin: hal.NoPin}}
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for sample without interval or ext_pin, got nil")
	}
	if !strings.Contains(err.Error(), "must have interval or ext_pin") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Error handling ---

func TestParse_InvalidDuration(t *testing.T) {
	input := []byte("interval = bad\n")
	cfg := Default()
	if err := Parse(input, &cfg); err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestParse_NegativeDuration(t *testing.T) {
	input := []byte("interval = -5s\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for negative duration, got nil")
	}
	if !strings.Contains(err.Error(), "negative duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_InvalidPin(t *testing.T) {
	input := []byte("ext_pin = abc\n")
	cfg := Default()
	if err := Parse(input, &cfg); err == nil {
		t.Fatal("expected error for invalid pin, got nil")
	}
}

func TestParse_PinOverflow(t *testing.T) {
	input := []byte("ext_pin = 256\n")
	cfg := Default()
	if err := Parse(input, &cfg); err == nil {
		t.Fatal("expected error for pin overflow, got nil")
	}
}

func TestParse_UnknownPayload(t *testing.T) {
	input := []byte("payload = mega\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown payload, got nil")
	}
	if !strings.Contains(err.Error(), "unknown payload") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_UnknownKey(t *testing.T) {
	input := []byte("bad_key = x\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_MultipleErrors(t *testing.T) {
	input := []byte("bad_key1 = x\nbad_key2 = y\n")
	cfg := Default()
	err := Parse(input, &cfg)
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

func TestParse_PayloadVariants(t *testing.T) {
	tests := []struct {
		value string
		want  Payload
	}{
		{"none", PayloadNone},
		{"min", PayloadMin},
		{"full", PayloadFull},
	}
	for _, tt := range tests {
		input := []byte("heartbeat = 1h\npayload = " + tt.value + "\n")
		cfg := Default()
		if err := Parse(input, &cfg); err != nil {
			t.Fatalf("Parse(payload=%s) error: %v", tt.value, err)
		}
		if cfg.Heartbeat.Payload != tt.want {
			t.Errorf("payload=%s: got %d, want %d", tt.value, cfg.Heartbeat.Payload, tt.want)
		}
	}
}

// --- Full example ---

func TestParse_FullExample(t *testing.T) {
	input := []byte(`
# Full flat config example
log_sinks = sd:error
data_sinks = sd
led_pin = 13

interval = 1m
sensors = temperature, humidity
ext_pin = 7

heartbeat = 1h
payload = full
blink_led = true
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Device
	if len(cfg.Device.LogSinks) != 1 || cfg.Device.LogSinks[0].Level != log.LevelError {
		t.Errorf("LogSinks = %v", cfg.Device.LogSinks)
	}
	if len(cfg.Device.DataSinks) != 1 || cfg.Device.DataSinks[0] != "sd" {
		t.Errorf("DataSinks = %v", cfg.Device.DataSinks)
	}
	if cfg.Device.LedPin != hal.Pin(13) {
		t.Errorf("LedPin = %d, want 13", cfg.Device.LedPin)
	}

	// Sample
	if cfg.Sample.Interval != time.Minute {
		t.Errorf("Sample.Interval = %v, want 1m", cfg.Sample.Interval)
	}
	if len(cfg.Sample.Sensors) != 2 {
		t.Errorf("Sample.Sensors = %v, want [temperature humidity]", cfg.Sample.Sensors)
	}
	if cfg.Sample.ExtPin != hal.Pin(7) {
		t.Errorf("Sample.ExtPin = %d, want 7", cfg.Sample.ExtPin)
	}

	// Heartbeat
	if cfg.Heartbeat.Interval != time.Hour {
		t.Errorf("Heartbeat.Interval = %v, want 1h", cfg.Heartbeat.Interval)
	}
	if cfg.Heartbeat.Payload != PayloadFull {
		t.Errorf("Heartbeat.Payload = %d, want PayloadFull", cfg.Heartbeat.Payload)
	}
	if !cfg.Heartbeat.BlinkLED {
		t.Error("Heartbeat.BlinkLED = false, want true")
	}
}
