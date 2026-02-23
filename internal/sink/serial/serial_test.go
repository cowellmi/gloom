package serial

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

var testTime = time.Date(2026, 2, 14, 9, 5, 7, 0, time.UTC)

func TestNilWriter_Record(t *testing.T) {
	s := NewSink(nil)
	ms := []sensor.Measurement{{Label: "temp", Value: "22", Unit: "C"}}
	if err := s.Record(testTime, "bme280", ms); err != nil {
		t.Fatalf("Record with nil writer: %v", err)
	}
}

func TestNilWriter_WriteLog(t *testing.T) {
	s := NewSink(nil)
	if err := s.WriteLog(testTime, log.LevelError, "fail"); err != nil {
		t.Fatalf("WriteLog with nil writer: %v", err)
	}
}

func TestRecord_FormatsLine(t *testing.T) {
	var buf bytes.Buffer
	s := NewSink(&buf)

	ms := []sensor.Measurement{{Label: "temp", Value: "22.5", Unit: "C"}}
	if err := s.Record(testTime, "bme280", ms); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	want := "[09:05:07] SEN | bme280: temp: 22.5 C\r\n"
	if got != want {
		t.Errorf("Record =\n  %q\nwant\n  %q", got, want)
	}
}

func TestRecord_MultipleMeasurements(t *testing.T) {
	var buf bytes.Buffer
	s := NewSink(&buf)

	ms := []sensor.Measurement{
		{Label: "temp", Value: "22", Unit: "C"},
		{Label: "hum", Value: "65", Unit: "%"},
		{Label: "pres", Value: "1013", Unit: "hPa"},
	}
	if err := s.Record(testTime, "bme280", ms); err != nil {
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

func TestWriteLog_Levels(t *testing.T) {
	tests := []struct {
		level log.Level
		tag   string
	}{
		{log.LevelDebug, "DBG"},
		{log.LevelInfo, "INF"},
		{log.LevelWarn, "WRN"},
		{log.LevelError, "ERR"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		s := NewSink(&buf)

		if err := s.WriteLog(testTime, tt.level, "hello"); err != nil {
			t.Fatalf("WriteLog(%s): %v", tt.tag, err)
		}

		got := buf.String()
		want := "[09:05:07] " + tt.tag + " | hello\r\n"
		if got != want {
			t.Errorf("WriteLog(%s) =\n  %q\nwant\n  %q", tt.tag, got, want)
		}
	}
}

func TestFlush_ReturnsNil(t *testing.T) {
	s := NewSink(nil)
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestName(t *testing.T) {
	s := NewSink(nil)
	if got := s.Name(); got != "serial" {
		t.Errorf("Name() = %q, want %q", got, "serial")
	}
}

func TestAppendTimestamp(t *testing.T) {
	tests := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "[00:00:00] "},
		{time.Date(2026, 1, 1, 9, 5, 7, 0, time.UTC), "[09:05:07] "},
		{time.Date(2026, 1, 1, 23, 59, 59, 0, time.UTC), "[23:59:59] "},
	}
	for _, tt := range tests {
		var buf [32]byte
		got := string(appendTimestamp(buf[:0], tt.t))
		if got != tt.want {
			t.Errorf("appendTimestamp(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
}

func TestAppendTwoDigits(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{5, "05"},
		{9, "09"},
		{10, "10"},
		{59, "59"},
	}
	for _, tt := range tests {
		var buf [4]byte
		got := string(appendTwoDigits(buf[:0], tt.n))
		if got != tt.want {
			t.Errorf("appendTwoDigits(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
