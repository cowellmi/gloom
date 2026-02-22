package log

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/debug"
)

// --- mocks ---

type mockSink struct {
	entries []string
	err     error
}

func (m *mockSink) WriteLog(_ time.Time, _ Level, msg string) error {
	m.entries = append(m.entries, msg)
	return m.err
}

func (m *mockSink) Flush() error { return m.err }

// --- tests ---

func TestLog_SinkErrorRoutedToDebug(t *testing.T) {
	var buf bytes.Buffer
	debug.Reset()
	debug.Add(&buf)
	defer debug.Reset()

	sink := &mockSink{err: errors.New("sd card full")}
	l := NewLogger(time.Time{})
	l.AddSink(sink, LevelDebug)

	l.Info("hello")

	out := buf.String()
	if !strings.Contains(out, "sink error: sd card full") {
		t.Errorf("expected debug output with sink error, got: %q", out)
	}
}

func TestLog_NoDebugOutputOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	debug.Reset()
	debug.Add(&buf)
	defer debug.Reset()

	sink := &mockSink{}
	l := NewLogger(time.Time{})
	l.AddSink(sink, LevelDebug)

	l.Info("hello")

	if buf.Len() != 0 {
		t.Errorf("expected no debug output, got: %q", buf.String())
	}
}

func TestLog_LevelFiltering(t *testing.T) {
	sink := &mockSink{}
	l := NewLogger(time.Time{})
	l.AddSink(sink, LevelWarn)

	l.Debug("skip")
	l.Info("skip")
	l.Warn("keep")
	l.Error("keep")

	if len(sink.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(sink.entries), sink.entries)
	}
	if sink.entries[0] != "keep" || sink.entries[1] != "keep" {
		t.Errorf("unexpected entries: %v", sink.entries)
	}
}
