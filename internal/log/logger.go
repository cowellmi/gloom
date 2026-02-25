// Package log provides leveled logging with per-sink minimum level
// filtering. The Logger fans out log entries to registered Sinks,
// skipping any sink whose minimum level exceeds the entry's level.
//
// The initial timestamp is provided at construction. The caller
// (typically the manager) caches the RTC time once per wake cycle
// and pushes updates via SetTime.
package log

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
)

// Sink receives log entries for output. Implementations decide their
// own serialization format and manage their own scratch buffers
// internally. Flush forces any buffered data to be written (called
// before sleep).
type Sink interface {
	Log(t time.Time, level config.LogLevel, msg string) error
	Flush() error
}

// target pairs a Sink with the minimum level it should receive.
type target struct {
	sink     Sink
	minLevel config.LogLevel
}

// Logger fans out log entries to sinks filtered by per-sink level.
type Logger struct {
	targets []target
	t       time.Time
}

// NewLogger creates a Logger with no sinks. now seeds the initial
// timestamp; the caller should pass the RTC time (or time.Now() as a
// fallback). Call AddSink to register output destinations.
func NewLogger(now time.Time, sinks ...Sink) *Logger {
	return &Logger{t: now}
}

// AddSink registers a Sink that will receive log entries at or above
// minLevel.
func (l *Logger) AddSink(s Sink, minLevel config.LogLevel) {
	l.targets = append(l.targets, target{sink: s, minLevel: minLevel})
}

// SetTime updates the timestamp used for subsequent log entries.
// The manager calls this once per wake cycle after reading the RTC.
func (l *Logger) SetTime(t time.Time) {
	l.t = t
}

// Log writes a log entry to all sinks whose minimum level is met.
func (l *Logger) Log(level config.LogLevel, msg string) {
	for i := range l.targets {
		if level >= l.targets[i].minLevel {
			err := l.targets[i].sink.Log(l.t, level, msg)
			if err != nil {
				// Route to debug (UART) instead of logging through
				// ourselves to avoid a recursive log loop.
				debug.Log("sink error: " + err.Error())
			}
		}
	}
}

// Write logs b at the given level, converting it to a string.
func (l *Logger) Write(b []byte, level config.LogLevel) {
	for i := range l.targets {
		if level >= l.targets[i].minLevel {
			if err := l.targets[i].sink.Log(l.t, level, string(b)); err != nil {
				debug.Log("sink error: " + err.Error())
			}
		}
	}
}

// Debug logs at LevelDebug.
func (l *Logger) Debug(msg string) { l.Log(config.LogLevelDebug, msg) }

// Info logs at LevelInfo.
func (l *Logger) Info(msg string) { l.Log(config.LogLevelInfo, msg) }

// Warn logs at LevelWarn.
func (l *Logger) Warn(msg string) { l.Log(config.LogLevelWarn, msg) }

// Error logs at LevelError.
func (l *Logger) Error(msg string) { l.Log(config.LogLevelError, msg) }

// Flush forces all sinks to write any buffered data. Called by the
// manager before entering sleep.
func (l *Logger) Flush() error {
	var errs []error
	for i := range l.targets {
		err := l.targets[i].sink.Flush()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
