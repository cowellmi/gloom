package log

import (
	"testing"
	"time"
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
