package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

// testDefault returns a Default config with standard test board values.
func testDefault() Config {
	return Default(hal.Pin(13), hal.NoPin, []string{"vbat"}, nil)
}

// --- Default ---

func TestDefault(t *testing.T) {
	cfg := testDefault()

	// Sinks
	serial, ok := cfg.Sinks["serial"]
	if !ok {
		t.Fatal("Sinks missing 'serial'")
	}
	if serial.LogLevel != LogLevelDebug {
		t.Errorf("serial log level = %v, want debug", serial.LogLevel)
	}
	blues, ok := cfg.Sinks["blues"]
	if !ok {
		t.Fatal("Sinks missing 'blues'")
	}
	if blues.LogLevel != LogLevelOff {
		t.Errorf("blues log level = %v, want off", blues.LogLevel)
	}
	sd, ok := cfg.Sinks["sd"]
	if !ok {
		t.Fatal("Sinks missing 'sd'")
	}
	if sd.LogLevel != LogLevelDebug {
		t.Errorf("sd log level = %v, want debug", sd.LogLevel)
	}

	// Pins
	if cfg.LEDPin != hal.Pin(13) {
		t.Errorf("LEDPin = %d, want 13", cfg.LEDPin)
	}
	if cfg.RTCIntPin != hal.NoPin {
		t.Errorf("RTCIntPin = %d, want NoPin", cfg.RTCIntPin)
	}

	// Groups
	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups len = %d, want 1", len(cfg.Groups))
	}
	g, ok := cfg.Groups["sample"]
	if !ok {
		t.Fatal("Groups missing 'sample'")
	}
	if g.Interval != 5*time.Second {
		t.Errorf("sample.Interval = %v, want 5s", g.Interval)
	}
	if len(g.Sensors) != 1 || g.Sensors[0] != "vbat" {
		t.Errorf("sample.Sensors = %v, want [vbat]", g.Sensors)
	}
	if !g.PulseLED {
		t.Error("sample.PulseLED = false, want true")
	}
}

// --- ParseMap ---

func TestParseMap_DeviceKeys(t *testing.T) {
	body := map[string]interface{}{
		"led_pin":             "16",
		"rtc_int_pin":         "7",
		"sd_chip_select_pins": "11, 10",
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if cfg.LEDPin != hal.Pin(16) {
		t.Errorf("LEDPin = %d, want 16", cfg.LEDPin)
	}
	if cfg.RTCIntPin != hal.Pin(7) {
		t.Errorf("RTCIntPin = %d, want 7", cfg.RTCIntPin)
	}
	if len(cfg.SDChipSelectPins) != 2 || cfg.SDChipSelectPins[0] != hal.Pin(11) || cfg.SDChipSelectPins[1] != hal.Pin(10) {
		t.Errorf("SDChipSelectPins = %v, want [11 10]", cfg.SDChipSelectPins)
	}
}

func TestParseMap_Sinks(t *testing.T) {
	body := map[string]interface{}{
		"sinks": map[string]interface{}{
			"serial": map[string]interface{}{"log_level": "warn"},
			"blues":  map[string]interface{}{"log_level": "error"},
			"sd":     map[string]interface{}{"log_level": "info"},
		},
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
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

func TestParseMap_SinksMerge(t *testing.T) {
	// Only "serial" updated; "blues" and "sd" keep defaults.
	body := map[string]interface{}{
		"sinks": map[string]interface{}{
			"serial": map[string]interface{}{"log_level": "warn"},
		},
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if cfg.Sinks["serial"].LogLevel != LogLevelWarn {
		t.Errorf("serial = %v, want warn", cfg.Sinks["serial"].LogLevel)
	}
	if cfg.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues should remain off, got %v", cfg.Sinks["blues"].LogLevel)
	}
	if cfg.Sinks["sd"].LogLevel != LogLevelDebug {
		t.Errorf("sd should remain debug, got %v", cfg.Sinks["sd"].LogLevel)
	}
}

func TestParseMap_SinksUnknownNameIgnored(t *testing.T) {
	// Unknown sink names are silently skipped (merge semantics).
	body := map[string]interface{}{
		"sinks": map[string]interface{}{
			"unknown": map[string]interface{}{"log_level": "warn"},
		},
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
}

func TestParseMap_SinksLogLevelOff(t *testing.T) {
	body := map[string]interface{}{
		"sinks": map[string]interface{}{
			"blues": map[string]interface{}{"log_level": "off"},
		},
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if cfg.Sinks["blues"].LogLevel != LogLevelOff {
		t.Errorf("blues = %v, want off", cfg.Sinks["blues"].LogLevel)
	}
}

func TestParseMap_Groups(t *testing.T) {
	body := map[string]interface{}{
		"groups": map[string]interface{}{
			"sample": map[string]interface{}{
				"interval":  "5m",
				"sensors":   "vbat, temp",
				"pulse_led": "true",
			},
			"rain": map[string]interface{}{
				"interrupt_pins": "7",
				"sensors":        "bucket",
			},
		},
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if len(cfg.Groups) != 2 {
		t.Fatalf("Groups len = %d, want 2", len(cfg.Groups))
	}

	sample, ok := cfg.Groups["sample"]
	if !ok {
		t.Fatal("Groups missing 'sample'")
	}
	if sample.Interval != 5*time.Minute {
		t.Errorf("sample.Interval = %v, want 5m", sample.Interval)
	}
	if len(sample.Sensors) != 2 || sample.Sensors[0] != "vbat" || sample.Sensors[1] != "temp" {
		t.Errorf("sample.Sensors = %v, want [vbat temp]", sample.Sensors)
	}
	if !sample.PulseLED {
		t.Error("sample.PulseLED = false, want true")
	}

	rain, ok := cfg.Groups["rain"]
	if !ok {
		t.Fatal("Groups missing 'rain'")
	}
	if len(rain.InterruptPins) != 1 || rain.InterruptPins[0] != hal.Pin(7) {
		t.Errorf("rain.InterruptPins = %v, want [7]", rain.InterruptPins)
	}
}

func TestParseMap_GroupsClearDefaults(t *testing.T) {
	body := map[string]interface{}{
		"groups": map[string]interface{}{
			"custom": map[string]interface{}{
				"interval": "1h",
			},
		},
	}
	cfg := testDefault() // has default "sample" group
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("Groups len = %d, want 1", len(cfg.Groups))
	}
	if _, ok := cfg.Groups["custom"]; !ok {
		t.Error("Groups missing 'custom'")
	}
	if _, ok := cfg.Groups["sample"]; ok {
		t.Error("default 'sample' group should have been cleared")
	}
}

func TestParseMap_RTCIntPin(t *testing.T) {
	body := map[string]interface{}{"rtc_int_pin": "5"}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
	}
	if cfg.RTCIntPin != hal.Pin(5) {
		t.Errorf("RTCIntPin = %d, want 5", cfg.RTCIntPin)
	}
}

func TestParseMap_NotehubInternalKeysSkipped(t *testing.T) {
	body := map[string]interface{}{
		"_tri_mins": "60",
	}
	cfg := testDefault()
	if err := ParseMap(&cfg, body); err != nil {
		t.Fatalf("ParseMap() error: %v", err)
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

func TestParseMap_GroupsNotObject(t *testing.T) {
	body := map[string]interface{}{"groups": "not-an-object"}
	cfg := testDefault()
	err := ParseMap(&cfg, body)
	if err == nil {
		t.Fatal("expected error for groups not object, got nil")
	}
	if !strings.Contains(err.Error(), "groups") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseMap_SinksNotObject(t *testing.T) {
	body := map[string]interface{}{"sinks": "not-an-object"}
	cfg := testDefault()
	err := ParseMap(&cfg, body)
	if err == nil {
		t.Fatal("expected error for sinks not object, got nil")
	}
	if !strings.Contains(err.Error(), "sinks") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Validate ---

func TestValidate_NoGroups(t *testing.T) {
	cfg := Config{}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no groups defined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupWithNoWakeSource(t *testing.T) {
	cfg := Config{Groups: map[string]Group{"s": {Sensors: []string{"vbat"}}}}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no wake source") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupWithInterval(t *testing.T) {
	cfg := Config{Groups: map[string]Group{"s": {Interval: 5 * time.Second}}}
	if err := Validate(&cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GroupWithInterruptPin(t *testing.T) {
	cfg := Config{Groups: map[string]Group{"rain": {InterruptPins: []hal.Pin{7}}}}
	if err := Validate(&cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MultiGroupAllNeedWakeSource(t *testing.T) {
	// Both groups have wake sources — valid.
	cfg := Config{Groups: map[string]Group{
		"s":  {Interval: 5 * time.Second},
		"hb": {Interval: time.Minute},
	}}
	if err := Validate(&cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MultiGroupOneWithoutWakeSource(t *testing.T) {
	// One group has no wake source — invalid (every group must have one).
	cfg := Config{Groups: map[string]Group{
		"s":  {Sensors: []string{"vbat"}}, // no interval, no pins
		"hb": {Interval: time.Minute},
	}}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected error when one group has no wake source, got nil")
	}
	if !strings.Contains(err.Error(), "no wake source") {
		t.Errorf("unexpected error: %v", err)
	}
}
