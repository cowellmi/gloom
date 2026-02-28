package sink

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/sensor"
)

var testTime = time.Date(2026, 2, 14, 9, 5, 7, 0, time.UTC)

func TestNilWriter_Record(t *testing.T) {
	s := NewSerial(nil)
	readings := []sensor.Reading{{Label: "temp", Value: 22, Unit: "C"}}
	if err := s.Data(testTime, "bme280", readings); err != nil {
		t.Fatalf("Record with nil writer: %v", err)
	}
}

func TestNilWriter_WriteLog(t *testing.T) {
	s := NewSerial(nil)
	if err := s.Log(testTime, config.LogLevelError, "fail"); err != nil {
		t.Fatalf("WriteLog with nil writer: %v", err)
	}
}

func TestRecord_FormatsLine(t *testing.T) {
	var buf bytes.Buffer
	s := NewSerial(&buf)

	readings := []sensor.Reading{{Label: "temp", Value: 22500, Unit: "mC"}}
	if err := s.Data(testTime, "bme280", readings); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "[09:05:07] SEN | bme280: temp: 22500 mC\r\n"
	if got != want {
		t.Errorf("Record =\n  %q\nwant\n  %q", got, want)
	}
}

func TestRecord_MultipleMeasurements(t *testing.T) {
	var buf bytes.Buffer
	s := NewSerial(&buf)

	readings := []sensor.Reading{
		{Label: "temp", Value: 22, Unit: "C"},
		{Label: "hum", Value: 65, Unit: "%"},
		{Label: "pres", Value: 1013, Unit: "hPa"},
	}
	if err := s.Data(testTime, "bme280", readings); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\r\n"), "\r\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "temp: 22 C") {
		t.Errorf("line 0 missing temp: %q", lines[0])
	}
	if !strings.Contains(lines[1], "hum: 65 %") {
		t.Errorf("line 1 missing hum: %q", lines[1])
	}
	if !strings.Contains(lines[2], "pres: 1013 hPa") {
		t.Errorf("line 2 missing pres: %q", lines[2])
	}
}

func TestRecord_NegativeValue_Serial(t *testing.T) {
	var buf bytes.Buffer
	s := NewSerial(&buf)

	readings := []sensor.Reading{{Label: "temp", Value: -5000, Unit: "mC"}}
	if err := s.Data(testTime, "ds18b20", readings); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "[09:05:07] SEN | ds18b20: temp: -5000 mC\r\n"
	if got != want {
		t.Errorf("Record =\n  %q\nwant\n  %q", got, want)
	}
}

func TestWriteLog_Levels(t *testing.T) {
	tests := []struct {
		level config.LogLevel
		tag   string
	}{
		{config.LogLevelDebug, "DBG"},
		{config.LogLevelInfo, "INF"},
		{config.LogLevelWarn, "WRN"},
		{config.LogLevelError, "ERR"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		s := NewSerial(&buf)

		if err := s.Log(testTime, tt.level, "hello"); err != nil {
			t.Fatalf("WriteLog(%s): %v", tt.tag, err)
		}

		got := buf.String()
		want := "[09:05:07] " + tt.tag + " | hello\r\n"
		if got != want {
			t.Errorf("WriteLog(%s) =\n  %q\nwant\n  %q", tt.tag, got, want)
		}
	}
}

func TestSerial_Flush_ReturnsNil(t *testing.T) {
	s := NewSerial(nil)
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}
