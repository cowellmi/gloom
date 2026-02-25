package log

import (
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/config"
)

// --- mocks ---

type mockSink struct {
	entries []string
	err     error
}

func (m *mockSink) Log(_ time.Time, _ config.LogLevel, msg string) error {
	m.entries = append(m.entries, msg)
	return m.err
}

func (m *mockSink) Flush() error { return m.err }

// --- tests ---

func TestLog_LevelFiltering(t *testing.T) {
	sink := &mockSink{}
	l := NewLogger(time.Time{})
	l.AddSink(sink, config.LogLevelWarn)

	l.Log(config.LogLevelDebug, "skip")
	l.Log(config.LogLevelInfo, "skip")
	l.Log(config.LogLevelWarn, "keep")
	l.Log(config.LogLevelError, "keep")

	if len(sink.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(sink.entries), sink.entries)
	}
	if sink.entries[0] != "keep" || sink.entries[1] != "keep" {
		t.Errorf("unexpected entries: %v", sink.entries)
	}
}
