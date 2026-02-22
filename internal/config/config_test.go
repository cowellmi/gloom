package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/log"
)

// --- Default ---

func TestDefault(t *testing.T) {
	cfg := Default()

	if len(cfg.Device.LogSinks) != 2 {
		t.Fatalf("LogSinks = %v, want 2 entries", cfg.Device.LogSinks)
	}
	if cfg.Device.LogSinks[0].Name != "uart" || cfg.Device.LogSinks[0].Level != log.LevelDebug {
		t.Errorf("LogSinks[0] = %+v, want uart:debug", cfg.Device.LogSinks[0])
	}
	wantSinks := []string{"uart", "usb"}
	if len(cfg.Device.DataSinks) != 2 || cfg.Device.DataSinks[0] != wantSinks[0] || cfg.Device.DataSinks[1] != wantSinks[1] {
		t.Errorf("DataSinks = %v, want %v", cfg.Device.DataSinks, wantSinks)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1", len(cfg.Groups))
	}
	g := cfg.Groups[0]
	if g.Name != "sample" {
		t.Errorf("Groups[0].Name = %q, want sample", g.Name)
	}
	if g.Interval != 5*time.Second {
		t.Errorf("Groups[0].Interval = %v, want 5s", g.Interval)
	}
	if len(g.Sensors) != 1 || g.Sensors[0] != "vbat" {
		t.Errorf("Groups[0].Sensors = %v, want [vbat]", g.Sensors)
	}
	if !g.PulseLED {
		t.Error("Groups[0].PulseLED = false, want true")
	}
}

// --- Device section ---

func TestParse_DeviceSection(t *testing.T) {
	input := []byte(`
[device]
log_sinks = uart:debug, sd:error
data_sinks = uart, usb, sd
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Device.LogSinks) != 2 {
		t.Fatalf("LogSinks = %v, want 2", cfg.Device.LogSinks)
	}
	if cfg.Device.LogSinks[1].Name != "sd" || cfg.Device.LogSinks[1].Level != log.LevelError {
		t.Errorf("LogSinks[1] = %+v, want sd:error", cfg.Device.LogSinks[1])
	}

	if len(cfg.Device.DataSinks) != 3 {
		t.Fatalf("DataSinks = %v, want 3 entries", cfg.Device.DataSinks)
	}
	if cfg.Device.DataSinks[2] != "sd" {
		t.Errorf("DataSinks[2] = %q, want sd", cfg.Device.DataSinks[2])
	}
}

func TestParse_LogSinksDefaultLevel(t *testing.T) {
	input := []byte(`
[device]
log_sinks = uart, usb
`)
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
	input := []byte(`
[device]
log_sinks = uart:verbose
`)
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "unknown log level") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_PulseLED(t *testing.T) {
	for _, val := range []string{"true", "True", "TRUE", "yes", "1"} {
		input := []byte("[sample]\ninterval = 5s\npulse_led = " + val)
		cfg := Config{}
		if err := Parse(input, &cfg); err != nil {
			t.Fatalf("Parse(pulse_led=%s) error: %v", val, err)
		}
		if !cfg.Groups[0].PulseLED {
			t.Errorf("pulse_led = %s: want true", val)
		}
	}

	input := []byte("[sample]\ninterval = 5s\npulse_led = false")
	cfg := Config{}
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse(pulse_led=false) error: %v", err)
	}
	if cfg.Groups[0].PulseLED {
		t.Error("pulse_led = false: want false")
	}
}

func TestParse_DeviceUnknownKey(t *testing.T) {
	input := []byte("[device]\nbad_key = 1\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown device key, got nil")
	}
	if !strings.Contains(err.Error(), "[device] unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Group parsing ---

func TestParse_MultipleGroups(t *testing.T) {
	input := []byte(`
[device]
data_sinks = uart

[weather]
interval = 1m
sensors = temp, humidity

[heartbeat]
interval = 1h
host = http://localhost:4000
payload = full
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2", len(cfg.Groups))
	}

	w := cfg.Groups[0]
	if w.Name != "weather" {
		t.Errorf("Groups[0].Name = %q, want weather", w.Name)
	}
	if w.Interval != time.Minute {
		t.Errorf("weather interval = %v, want 1m", w.Interval)
	}
	if len(w.Sensors) != 2 || w.Sensors[0] != "temp" || w.Sensors[1] != "humidity" {
		t.Errorf("weather sensors = %v, want [temp humidity]", w.Sensors)
	}

	h := cfg.Groups[1]
	if h.Name != "heartbeat" {
		t.Errorf("Groups[1].Name = %q, want heartbeat", h.Name)
	}
	if h.Interval != time.Hour {
		t.Errorf("heartbeat interval = %v, want 1h", h.Interval)
	}
	if h.Host != "http://localhost:4000" {
		t.Errorf("heartbeat host = %q", h.Host)
	}
	if h.Payload != PayloadFull {
		t.Errorf("heartbeat payload = %d, want PayloadFull", h.Payload)
	}
}

func TestParse_ExternalIntPin(t *testing.T) {
	input := []byte(`
[device]
data_sinks = sd

[rain]
external_int_pin = 7
sensors = tipping_bucket
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	g := cfg.Groups[0]
	if g.ExternalIntPin != 7 {
		t.Errorf("ExternalIntPin = %d, want 7", g.ExternalIntPin)
	}
	if g.Interval != 0 {
		t.Errorf("Interval = %v, want 0", g.Interval)
	}
}

func TestParse_GroupWithBothTriggers(t *testing.T) {
	input := []byte(`
[device]
data_sinks = sd

[rain]
interval = 5m
external_int_pin = 7
sensors = tipping_bucket
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	g := cfg.Groups[0]
	if g.Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want 5m", g.Interval)
	}
	if g.ExternalIntPin != 7 {
		t.Errorf("ExternalIntPin = %d, want 7", g.ExternalIntPin)
	}
}

func TestParse_RepeatedSection(t *testing.T) {
	input := []byte(`
[device]
data_sinks = uart

[weather]
interval = 1m

[weather]
sensors = temp
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1", len(cfg.Groups))
	}
	g := cfg.Groups[0]
	if g.Interval != time.Minute {
		t.Errorf("Interval = %v, want 1m", g.Interval)
	}
	if len(g.Sensors) != 1 || g.Sensors[0] != "temp" {
		t.Errorf("Sensors = %v, want [temp]", g.Sensors)
	}
}

// --- Inline comments ---

func TestParse_InlineComments(t *testing.T) {
	input := []byte(`
[device]
data_sinks = uart

[weather]
interval = 5m # every five minutes
sensors = temp
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.Groups[0].Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want 5m (inline comment should be stripped)", cfg.Groups[0].Interval)
	}
}

func TestParse_PayloadInlineComment(t *testing.T) {
	input := []byte(`
[heartbeat]
interval = 1h
payload = full # none | full | min
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.Groups[0].Payload != PayloadFull {
		t.Errorf("Payload = %d, want PayloadFull", cfg.Groups[0].Payload)
	}
}

func TestParse_URLWithFragment(t *testing.T) {
	input := []byte(`
[heartbeat]
interval = 1h
host = http://example.com/path#section
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.Groups[0].Host != "http://example.com/path#section" {
		t.Errorf("Host = %q, want URL with fragment preserved", cfg.Groups[0].Host)
	}
}

// --- Comments and blanks ---

func TestParse_CommentsAndBlanks(t *testing.T) {
	input := []byte(`
# This is a comment

[device]
# Another comment
data_sinks = uart

[sample]
interval = 3s
sensors = vbat
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Device.DataSinks) != 1 || cfg.Device.DataSinks[0] != "uart" {
		t.Errorf("DataSinks = %v, want [uart]", cfg.Device.DataSinks)
	}
	if cfg.Groups[0].Interval != 3*time.Second {
		t.Errorf("Interval = %v, want 3s", cfg.Groups[0].Interval)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	cfg := Default()
	if err := Parse([]byte(""), &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Device.LogSinks) != 2 {
		t.Error("LogSinks should retain defaults from Default()")
	}
	if len(cfg.Groups) != 0 {
		t.Errorf("Groups = %d, want 0 (empty config defines no groups)", len(cfg.Groups))
	}
}

// --- Validation ---

func TestParse_GroupMissingTrigger(t *testing.T) {
	input := []byte(`
[device]
data_sinks = uart

[weather]
sensors = temp
`)
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for group without interval or ext pin, got nil")
	}
	if !strings.Contains(err.Error(), "must have interval or external_int_pin") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_SensorsWithoutDeviceDataSinks(t *testing.T) {
	input := []byte(`
[device]
data_sinks =

[weather]
interval = 1m
sensors = temp
`)
	cfg := Config{}
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for sensors without device data sinks, got nil")
	}
	if !strings.Contains(err.Error(), "sensors require at least one data_sink") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_GroupNoSensorsNoSinks_OK(t *testing.T) {
	input := []byte(`
[heartbeat]
interval = 1h
host = http://example.com
payload = full
`)
	cfg := Config{}
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	g := cfg.Groups[0]
	if len(g.Sensors) != 0 {
		t.Errorf("Sensors = %v, want empty", g.Sensors)
	}
}

// --- Error handling ---

func TestParse_InvalidDuration(t *testing.T) {
	input := []byte("[sample]\ninterval = bad\nsensors = x\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestParse_NegativeDuration(t *testing.T) {
	input := []byte("[sample]\ninterval = -5s\nsensors = x\n")
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
	input := []byte("[rain]\nexternal_int_pin = abc\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for invalid pin, got nil")
	}
}

func TestParse_PinOverflow(t *testing.T) {
	input := []byte("[rain]\nexternal_int_pin = 256\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for pin overflow, got nil")
	}
}

func TestParse_UnknownPayload(t *testing.T) {
	input := []byte("[heartbeat]\ninterval = 1h\npayload = mega\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown payload, got nil")
	}
	if !strings.Contains(err.Error(), "unknown payload") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_UnknownGroupKey(t *testing.T) {
	input := []byte("[weather]\ninterval = 1m\nbad_key = x\nsensors = temp\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for unknown group key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key: bad_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_KeyOutsideSection(t *testing.T) {
	input := []byte("interval = 1m\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for key outside section, got nil")
	}
	if !strings.Contains(err.Error(), "key outside of section") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_MultipleErrors(t *testing.T) {
	input := []byte("[device]\nbad_key1 = x\nbad_key2 = y\n")
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

func TestParse_EmptySectionName(t *testing.T) {
	input := []byte("[]\ninterval = 1m\n")
	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("expected error for empty section name, got nil")
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
		input := []byte("[hb]\ninterval = 1h\npayload = " + tt.value + "\n")
		cfg := Default()
		if err := Parse(input, &cfg); err != nil {
			t.Fatalf("Parse(payload=%s) error: %v", tt.value, err)
		}
		if cfg.Groups[0].Payload != tt.want {
			t.Errorf("payload=%s: got %d, want %d", tt.value, cfg.Groups[0].Payload, tt.want)
		}
	}
}

// --- Full example ---

func TestParse_FullExample(t *testing.T) {
	input := []byte(`
[device]
log_sinks = uart:debug, usb:debug, sd:error
data_sinks = uart, usb, sd

[weather]
interval = 1m
sensors = temperature, humidity
pulse_led = true

[rain]
external_int_pin = 7
sensors = tipping_bucket

[heartbeat]
interval = 1h
host = http://localhost:4000/heartbeat
payload = full
`)
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Device
	if len(cfg.Device.LogSinks) != 3 {
		t.Fatalf("LogSinks = %d, want 3", len(cfg.Device.LogSinks))
	}
	if len(cfg.Device.DataSinks) != 3 {
		t.Fatalf("DataSinks = %d, want 3", len(cfg.Device.DataSinks))
	}

	// Groups
	if len(cfg.Groups) != 3 {
		t.Fatalf("Groups = %d, want 3", len(cfg.Groups))
	}

	// Weather
	w := cfg.Groups[0]
	if w.Interval != time.Minute {
		t.Errorf("weather interval = %v, want 1m", w.Interval)
	}
	if len(w.Sensors) != 2 {
		t.Errorf("weather sensors = %v, want [temperature humidity]", w.Sensors)
	}
	if !w.PulseLED {
		t.Error("weather pulse_led = false, want true")
	}

	// Rain has ext pin, no pulse_led
	r := cfg.Groups[1]
	if r.ExternalIntPin != 7 {
		t.Errorf("rain ext pin = %d, want 7", r.ExternalIntPin)
	}
	if r.PulseLED {
		t.Error("rain pulse_led = true, want false")
	}

	// Heartbeat
	h := cfg.Groups[2]
	if h.Interval != time.Hour {
		t.Errorf("heartbeat interval = %v, want 1h", h.Interval)
	}
	if h.Payload != PayloadFull {
		t.Errorf("heartbeat payload = %d, want PayloadFull", h.Payload)
	}
	if h.Host != "http://localhost:4000/heartbeat" {
		t.Errorf("heartbeat host = %q", h.Host)
	}
}
