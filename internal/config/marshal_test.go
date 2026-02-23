package config

import (
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
)

func TestMarshal_RoundTrip(t *testing.T) {
	orig := Config{
		Device: Device{
			LogSinks: []LogSinkEntry{
				{Name: "serial", Level: log.LevelDebug},
				{Name: "sd", Level: log.LevelError},
			},
			DataSinks: []string{"serial", "sd"},
		},
		Groups: []Group{
			{
				Name:     "weather",
				Interval: time.Minute,
				Sensors:  []string{"temperature", "humidity"},
				PulseLED: true,
			},
			{
				Name:     "heartbeat",
				Interval: time.Hour,
				Host:     "http://example.com/hb",
				Payload:  PayloadFull,
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
	if got.Device.LogSinks[0].Name != "serial" || got.Device.LogSinks[0].Level != log.LevelDebug {
		t.Errorf("LogSink[0] = %+v", got.Device.LogSinks[0])
	}
	if got.Device.LogSinks[1].Name != "sd" || got.Device.LogSinks[1].Level != log.LevelError {
		t.Errorf("LogSink[1] = %+v", got.Device.LogSinks[1])
	}
	if len(got.Device.DataSinks) != 2 || got.Device.DataSinks[0] != "serial" {
		t.Errorf("DataSinks = %v", got.Device.DataSinks)
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
				Name:           "minimal",
				Interval:       5 * time.Second,
				ExternalIntPin: hal.NoPin,
			},
		},
	}

	data, err := cfg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	s := string(data)
	for _, absent := range []string{
		"sensors", "pulse_led", "host", "payload",
		"external_int_pin",
	} {
		if strings.Contains(s, absent) {
			t.Errorf("output should not contain %q:\n%s", absent, s)
		}
	}
}

func TestMarshal_ExternalIntPin(t *testing.T) {
	cfg := Config{
		Groups: []Group{
			{Name: "rain", ExternalIntPin: hal.Pin(7), Sensors: []string{"bucket"}},
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
