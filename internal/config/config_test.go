package config

import (
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.SampleInterval != 5*time.Second {
		t.Errorf("SampleInterval = %v, want 5s", cfg.SampleInterval)
	}
	if cfg.HeartbeatInterval != 0 {
		t.Errorf("HeartbeatInterval = %v, want 0 (disabled)", cfg.HeartbeatInterval)
	}
	if !cfg.SerialEnabled {
		t.Error("SerialEnabled = false, want true")
	}
	if !cfg.LedEnabled {
		t.Error("LedEnabled = false, want true")
	}
	if len(cfg.Sensors) != 0 {
		t.Errorf("Sensors = %v, want empty", cfg.Sensors)
	}
}

func TestParse_ValidConfig(t *testing.T) {
	input := []byte(`
sample_interval = 10s
heartbeat_interval = 1m
serial = false
sensors = temp, humidity, pressure
`)

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.SampleInterval != 10*time.Second {
		t.Errorf("SampleInterval = %v, want 10s", cfg.SampleInterval)
	}
	if cfg.HeartbeatInterval != time.Minute {
		t.Errorf("HeartbeatInterval = %v, want 1m", cfg.HeartbeatInterval)
	}
	if cfg.SerialEnabled {
		t.Error("SerialEnabled = true, want false")
	}

	want := []string{"temp", "humidity", "pressure"}
	if len(cfg.Sensors) != len(want) {
		t.Fatalf("Sensors = %v, want %v", cfg.Sensors, want)
	}
	for i, s := range cfg.Sensors {
		if s != want[i] {
			t.Errorf("Sensors[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestParse_CommentsAndBlanks(t *testing.T) {
	input := []byte(`
# This is a comment
sample_interval = 3s

# Another comment

serial = false
`)

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.SampleInterval != 3*time.Second {
		t.Errorf("SampleInterval = %v, want 3s", cfg.SampleInterval)
	}
	if cfg.SerialEnabled {
		t.Error("SerialEnabled = true, want false")
	}
}

func TestParse_EmptyInput(t *testing.T) {
	cfg := Default()
	if err := Parse([]byte(""), &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// All defaults should be preserved.
	def := Default()
	if cfg.SampleInterval != def.SampleInterval {
		t.Errorf("SampleInterval changed from default")
	}
}

func TestParse_InvalidDuration(t *testing.T) {
	input := []byte("sample_interval = not_a_duration")

	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("Parse() expected error for invalid duration, got nil")
	}
}

func TestParse_SensorsWhitespace(t *testing.T) {
	input := []byte("sensors =  a ,  b  , c ")

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	want := []string{"a", "b", "c"}
	if len(cfg.Sensors) != len(want) {
		t.Fatalf("Sensors = %v, want %v", cfg.Sensors, want)
	}
	for i, s := range cfg.Sensors {
		if s != want[i] {
			t.Errorf("Sensors[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestParse_SensorsEmpty(t *testing.T) {
	input := []byte("sensors = ")

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(cfg.Sensors) != 0 {
		t.Errorf("Sensors = %v, want empty", cfg.Sensors)
	}
}

func TestParse_PartialOverride(t *testing.T) {
	input := []byte("sample_interval = 30s")

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if cfg.SampleInterval != 30*time.Second {
		t.Errorf("SampleInterval = %v, want 30s", cfg.SampleInterval)
	}
	// Other fields should keep defaults.
	if !cfg.SerialEnabled {
		t.Error("SerialEnabled changed from default")
	}
}

func TestParse_LineWithoutEquals(t *testing.T) {
	input := []byte("no_equals_here")

	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Should be silently ignored, defaults preserved.
	def := Default()
	if cfg.SampleInterval != def.SampleInterval {
		t.Errorf("SampleInterval changed from default")
	}
}

func TestParse_MultipleErrors(t *testing.T) {
	input := []byte("sample_interval = bad\nheartbeat_interval = also_bad\n")

	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("Parse() expected errors, got nil")
	}

	// errors.Join separates with newlines; both errors must be present.
	msg := err.Error()
	if !strings.Contains(msg, "bad") {
		t.Errorf("error should mention first bad value, got: %s", msg)
	}
	if !strings.Contains(msg, "also_bad") {
		t.Errorf("error should mention second bad value, got: %s", msg)
	}
}

func TestParse_UnknownKey(t *testing.T) {
	input := []byte("sampl_interval = 10s\n")

	cfg := Default()
	err := Parse(input, &cfg)
	if err == nil {
		t.Fatal("Parse() expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config key: sampl_interval") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_SensorsResetOnReparse(t *testing.T) {
	cfg := Default()

	first := []byte("sensors = a, b")
	if err := Parse(first, &cfg); err != nil {
		t.Fatalf("first Parse() error: %v", err)
	}
	if len(cfg.Sensors) != 2 {
		t.Fatalf("after first parse: Sensors = %v, want [a b]", cfg.Sensors)
	}

	second := []byte("sensors = x")
	if err := Parse(second, &cfg); err != nil {
		t.Fatalf("second Parse() error: %v", err)
	}

	want := []string{"x"}
	if len(cfg.Sensors) != len(want) {
		t.Fatalf("after second parse: Sensors = %v, want %v", cfg.Sensors, want)
	}
	if cfg.Sensors[0] != "x" {
		t.Errorf("Sensors[0] = %q, want %q", cfg.Sensors[0], "x")
	}
}

func TestParse_EnableLED(t *testing.T) {
	input := []byte("enable_led = false")
	cfg := Default()
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if cfg.LedEnabled {
		t.Error("LedEnabled = true, want false")
	}

	input = []byte("enable_led = true")
	cfg = Default()
	cfg.LedEnabled = false // start with false
	if err := Parse(input, &cfg); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if !cfg.LedEnabled {
		t.Error("LedEnabled = false, want true")
	}
}

func TestParse_NegativeDurations(t *testing.T) {
	tests := []struct {
		key   string
		input string
	}{
		{"sample_interval", "sample_interval = -5s"},
		{"heartbeat_interval", "heartbeat_interval = -1m"},
	}

	for _, tt := range tests {
		cfg := Default()
		err := Parse([]byte(tt.input), &cfg)
		if err == nil {
			t.Errorf("Parse(%q) expected error for negative duration, got nil", tt.key)
			continue
		}
	}
}

func TestParse_ZeroDuration(t *testing.T) {
	// Zero means "disabled" for all duration fields -- should be accepted.
	keys := []string{
		"sample_interval = 0s",
		"heartbeat_interval = 0s",
	}
	for _, input := range keys {
		cfg := Default()
		if err := Parse([]byte(input), &cfg); err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", input, err)
		}
	}
}
