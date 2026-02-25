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
	"github.com/cowellmi/gloom/internal/sink"
)

// target pairs a Sink with the minimum level it should receive.
type target struct {
	sink     sink.LogSink
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
func NewLogger(now time.Time, sinks ...sink.LogSink) *Logger {
	return &Logger{t: now}
}

// AddSink registers a Sink that will receive log entries at or above
// minLevel.
func (l *Logger) AddSink(s sink.LogSink, minLevel config.LogLevel) {
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

type wrapped interface{ Unwrap() []error }

func (l *Logger) LogError(level config.LogLevel, err error, prefix string) {
	if wErr, ok := err.(wrapped); ok {
		for _, iErr := range wErr.Unwrap() {
			l.Log(level, prefix+iErr.Error())
		}
	} else {
		l.Log(level, prefix+err.Error())
	}
}

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
