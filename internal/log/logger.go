// Package log provides leveled logging with per-sink minimum level
// filtering. The Logger fans out log entries to registered Sinks,
// skipping any sink whose minimum level exceeds the entry's level.
//
// Timestamps are set explicitly via SetTime rather than read from a
// clock internally -- the caller (typically the manager) caches the
// RTC time once per wake cycle and pushes it into the Logger.
package log

import "time"

// Level represents log severity. Values mirror slog conventions.
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// Sink receives log entries for output. Implementations decide their
// own serialization format. Flush forces any buffered data to be
// written (called before sleep).
type Sink interface {
	WriteLog(buf []byte, t time.Time, level Level, msg string) error
	Flush() error
}

// target pairs a Sink with the minimum level it should receive.
type target struct {
	sink     Sink
	minLevel Level
}

// Size of the scratch buffer used by the Logger for formatting.
const logBufSize = 256

// Logger fans out log entries to sinks filtered by per-sink level.
type Logger struct {
	targets []target
	t       time.Time
	buf     [logBufSize]byte
}

// New creates a Logger with no sinks and the current time. Call AddSink
// to register output destinations.
func NewLogger() *Logger {
	return &Logger{t: time.Now()}
}

// AddSink registers a Sink that will receive log entries at or above
// minLevel.
func (l *Logger) AddSink(s Sink, minLevel Level) {
	l.targets = append(l.targets, target{sink: s, minLevel: minLevel})
}

// SetTime updates the timestamp used for subsequent log entries.
// The manager calls this once per wake cycle after reading the RTC.
func (l *Logger) SetTime(t time.Time) {
	l.t = t
}

// Log writes a log entry to all sinks whose minimum level is met.
func (l *Logger) Log(level Level, msg string) {
	for i := range l.targets {
		if level >= l.targets[i].minLevel {
			// Sink errors are silently ignored. Sinks self-disable
			// on persistent write failures.
			l.targets[i].sink.WriteLog(l.buf[:0], l.t, level, msg)
		}
	}
}

// Debug logs at LevelDebug.
func (l *Logger) Debug(msg string) { l.Log(LevelDebug, msg) }

// Info logs at LevelInfo.
func (l *Logger) Info(msg string) { l.Log(LevelInfo, msg) }

// Warn logs at LevelWarn.
func (l *Logger) Warn(msg string) { l.Log(LevelWarn, msg) }

// Error logs at LevelError.
func (l *Logger) Error(msg string) { l.Log(LevelError, msg) }

// Flush forces all sinks to write any buffered data. Called by the
// manager before entering sleep.
func (l *Logger) Flush() {
	for i := range l.targets {
		l.targets[i].sink.Flush()
	}
}
