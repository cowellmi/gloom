package sink

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// memFile implements AppendFile backed by a bytes.Buffer.
type memFile struct {
	buf    bytes.Buffer
	synced bool
	closed bool
}

func (f *memFile) Write(p []byte) (int, error) { return f.buf.Write(p) }
func (f *memFile) Sync() error                 { f.synced = true; return nil }
func (f *memFile) Close() error                { f.closed = true; return nil }

// memOpener tracks files opened by name so tests can inspect contents.
type memOpener struct {
	files map[string]*memFile
}

func newMemOpener() *memOpener {
	return &memOpener{files: make(map[string]*memFile)}
}

func (o *memOpener) Open(name string) (AppendFile, error) {
	f := &memFile{}
	o.files[name] = f
	return f, nil
}

func fileNames(o *memOpener) []string {
	names := make([]string, 0, len(o.files))
	for n := range o.files {
		names = append(names, n)
	}
	return names
}

func TestNew_OpensFilesForDate(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	_, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{"logs", ".log"}, now)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := o.files["data/20260214.csv"]; !ok {
		t.Errorf("expected data/20260214.csv, got files: %v", fileNames(o))
	}
	if _, ok := o.files["logs/20260214.log"]; !ok {
		t.Errorf("expected logs/20260214.log, got files: %v", fileNames(o))
	}
}

func TestRecord_WritesCSV(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 30, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{}, now)
	if err != nil {
		t.Fatal(err)
	}

	readings := []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}}
	if err := s.Record(now, "bme280", readings); err != nil {
		t.Fatal(err)
	}

	f := o.files["data/20260214.csv"]
	got := f.buf.String()
	want := "2026-02-14T10:30:00,bme280,temp,22000,mC\n"
	if got != want {
		t.Errorf("CSV row =\n  %q\nwant\n  %q", got, want)
	}
}

func TestRecord_NegativeValue(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 30, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{}, now)
	if err != nil {
		t.Fatal(err)
	}

	readings := []sensor.Reading{{Label: "temp", Value: -5000, Unit: "mC"}}
	if err := s.Record(now, "ds18b20", readings); err != nil {
		t.Fatal(err)
	}

	f := o.files["data/20260214.csv"]
	got := f.buf.String()
	want := "2026-02-14T10:30:00,ds18b20,temp,-5000,mC\n"
	if got != want {
		t.Errorf("CSV row =\n  %q\nwant\n  %q", got, want)
	}
}

func TestWriteLog_WritesEntry(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 30, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{}, FileSpec{"logs", ".log"}, now)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.WriteLog(now, log.LevelError, "disk full"); err != nil {
		t.Fatal(err)
	}

	f := o.files["logs/20260214.log"]
	got := f.buf.String()
	want := "2026-02-14T10:30:00 ERR disk full\n"
	if got != want {
		t.Errorf("log line =\n  %q\nwant\n  %q", got, want)
	}
}

func TestRotation_OnDateChange(t *testing.T) {
	o := newMemOpener()
	day1 := time.Date(2026, 2, 14, 23, 59, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{"logs", ".log"}, day1)
	if err != nil {
		t.Fatal(err)
	}

	readings := []sensor.Reading{{Label: "temp", Value: 20000, Unit: "mC"}}
	if err := s.Record(day1, "dev", readings); err != nil {
		t.Fatal(err)
	}

	day2 := time.Date(2026, 2, 15, 0, 1, 0, 0, time.UTC)
	if err := s.WriteLog(day2, log.LevelInfo, "new day"); err != nil {
		t.Fatal(err)
	}

	day1Data := o.files["data/20260214.csv"]
	day1Log := o.files["logs/20260214.log"]
	if !day1Data.closed {
		t.Error("day1 data file was not closed on rotation")
	}
	if !day1Log.closed {
		t.Error("day1 log file was not closed on rotation")
	}

	if _, ok := o.files["data/20260215.csv"]; !ok {
		t.Errorf("expected data/20260215.csv after rotation, got files: %v", fileNames(o))
	}
	day2Log := o.files["logs/20260215.log"]
	if day2Log == nil {
		t.Fatalf("expected logs/20260215.log after rotation, got files: %v", fileNames(o))
	}
	if !strings.Contains(day2Log.buf.String(), "new day") {
		t.Errorf("day2 log missing entry, got: %q", day2Log.buf.String())
	}
}

func TestRotation_SameDateNoReopen(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{}, now)
	if err != nil {
		t.Fatal(err)
	}

	readings := []sensor.Reading{{Label: "a", Value: 1, Unit: ""}}
	_ = s.Record(now, "dev", readings)

	later := now.Add(2 * time.Hour)
	readings[0].Value = 2
	_ = s.Record(later, "dev", readings)

	if len(o.files) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(o.files), fileNames(o))
	}

	f := o.files["data/20260214.csv"]
	lines := strings.Split(strings.TrimSpace(f.buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 CSV lines, got %d", len(lines))
	}
}

func TestFlush_SyncsOpenFiles(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{"logs", ".log"}, now)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	if !o.files["data/20260214.csv"].synced {
		t.Error("data file was not synced on Flush")
	}
	if !o.files["logs/20260214.log"].synced {
		t.Error("log file was not synced on Flush")
	}
}

func TestEmptyDir_SkipsFile(t *testing.T) {
	o := newMemOpener()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)

	s, err := NewRotaryFileSink("test", o.Open, FileSpec{"data", ".csv"}, FileSpec{}, now)
	if err != nil {
		t.Fatal(err)
	}

	if len(o.files) != 1 {
		t.Errorf("expected 1 file, got %d: %v", len(o.files), fileNames(o))
	}

	if err := s.WriteLog(now, log.LevelInfo, "hi"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFilename(t *testing.T) {
	tests := []struct {
		spec FileSpec
		t    time.Time
		want string
	}{
		{FileSpec{"data", ".csv"}, time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC), "data/20260214.csv"},
		{FileSpec{"logs", ".log"}, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), "logs/20251201.log"},
		{FileSpec{"out", ".txt"}, time.Date(2030, 1, 9, 0, 0, 0, 0, time.UTC), "out/20300109.txt"},
	}
	for _, tt := range tests {
		got := buildFilename(tt.spec, tt.t)
		if got != tt.want {
			t.Errorf("buildFilename(%v, %v) = %q, want %q", tt.spec, tt.t, got, tt.want)
		}
	}
}
