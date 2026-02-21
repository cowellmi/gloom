package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/log"
)

func TestMarshal_RoundTrip(t *testing.T) {
	orig := Config{
		Device: Device{
			LogSinks: []LogSinkEntry{
				{Name: "uart", Level: log.LevelDebug},
				{Name: "sd", Level: log.LevelError},
			},
			DataSinks:  []string{"uart", "sd"},
			LedPin:     13,
			RTCWakePin: 12,
			Rails: []RailConfig{
				{Name: "3v3", Pin: 5, ActiveLow: true, Always: true},
				{Name: "5v", Pin: 6, ActiveLow: false, Always: false},
			},
		},
		Groups: []Group{
			{
				Name:     "weather",
				Interval: time.Minute,
				Sensors:  []string{"temperature", "humidity"},
				Rails:    []string{"5v"},
				PulseLED: true,
			},
			{
				Name:    "heartbeat",
				Interval: time.Hour,
				Host:    "http://example.com/hb",
				Payload: PayloadFull,
			},
		},
	}

	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var got Config
	if err := Parse(data, &got); err != nil {
		t.Fatalf("Parse(Marshal()) error: %v\nINI:\n%s", err, data)
	}

	// Device
	if len(got.Device.LogSinks) != 2 {
		t.Errorf("LogSinks = %d, want 2", len(got.Device.LogSinks))
	}
	if got.Device.LogSinks[0].Name != "uart" || got.Device.LogSinks[0].Level != log.LevelDebug {
		t.Errorf("LogSink[0] = %+v", got.Device.LogSinks[0])
	}
	if got.Device.LogSinks[1].Name != "sd" || got.Device.LogSinks[1].Level != log.LevelError {
		t.Errorf("LogSink[1] = %+v", got.Device.LogSinks[1])
	}
	if len(got.Device.DataSinks) != 2 || got.Device.DataSinks[0] != "uart" {
		t.Errorf("DataSinks = %v", got.Device.DataSinks)
	}
	if got.Device.LedPin != 13 {
		t.Errorf("LedPin = %d, want 13", got.Device.LedPin)
	}
	if got.Device.RTCWakePin != 12 {
		t.Errorf("RTCWakePin = %d, want 12", got.Device.RTCWakePin)
	}

	// Rails
	if len(got.Device.Rails) != 2 {
		t.Fatalf("Rails = %d, want 2", len(got.Device.Rails))
	}
	r0 := got.Device.Rails[0]
	if r0.Name != "3v3" || r0.Pin != 5 || !r0.ActiveLow || !r0.Always {
		t.Errorf("Rail[0] = %+v", r0)
	}
	r1 := got.Device.Rails[1]
	if r1.Name != "5v" || r1.Pin != 6 || r1.ActiveLow || r1.Always {
		t.Errorf("Rail[1] = %+v", r1)
	}

	// Groups
	if len(got.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2", len(got.Groups))
	}

	w := got.Groups[0]
	if w.Name != "weather" {
		t.Errorf("group 0 name = %q", w.Name)
	}
	if w.Interval != time.Minute {
		t.Errorf("weather interval = %v, want 1m", w.Interval)
	}
	if len(w.Sensors) != 2 {
		t.Errorf("weather sensors = %v", w.Sensors)
	}
	if len(w.Rails) != 1 || w.Rails[0] != "5v" {
		t.Errorf("weather rails = %v, want [5v]", w.Rails)
	}
	if !w.PulseLED {
		t.Error("weather pulse_led = false, want true")
	}

	h := got.Groups[1]
	if h.Interval != time.Hour {
		t.Errorf("heartbeat interval = %v, want 1h", h.Interval)
	}
	if h.Host != "http://example.com/hb" {
		t.Errorf("heartbeat host = %q", h.Host)
	}
	if h.Payload != PayloadFull {
		t.Errorf("heartbeat payload = %d, want PayloadFull", h.Payload)
	}
}

func TestMarshal_ZeroFieldsOmitted(t *testing.T) {
	cfg := Config{
		Groups: []Group{
			{
				Name:     "minimal",
				Interval: 5 * time.Second,
			},
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	s := string(data)
	for _, absent := range []string{
		"led_pin", "rtc_wake_pin",
		"uart_tx_pin", "uart_rx_pin",
		"sensors", "rails", "pulse_led", "host", "payload",
		"external_int_pin", "[rails]",
	} {
		if strings.Contains(s, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, s)
		}
	}
}

func TestMarshal_NoRails(t *testing.T) {
	cfg := Config{
		Device: Device{
			LogSinks:  []LogSinkEntry{{Name: "uart", Level: log.LevelDebug}},
			DataSinks: []string{"uart"},
		},
		Groups: []Group{
			{Name: "test", Interval: 10 * time.Second, Sensors: []string{"fake"}},
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if strings.Contains(string(data), "[rails]") {
		t.Errorf("output should not contain [rails] when no rails configured:\n%s", data)
	}
}

func TestMarshal_ExternalIntPin(t *testing.T) {
	cfg := Config{
		Groups: []Group{
			{Name: "rain", ExternalIntPin: 7, Sensors: []string{"bucket"}},
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	if !strings.Contains(string(data), "external_int_pin = 7") {
		t.Errorf("expected external_int_pin = 7 in output:\n%s", data)
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
			Groups: []Group{{Name: "t", Interval: tt.d}},
		}
		data, err := cfg.Marshal()
		if err != nil {
			t.Fatalf("Marshal() error for %v: %v", tt.d, err)
		}
		if !strings.Contains(string(data), "interval = "+tt.want) {
			t.Errorf("duration %v: want %q in output:\n%s", tt.d, tt.want, data)
		}
	}
}
