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
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/debug"
)

// Level represents log severity. Values mirror slog conventions.
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
	LevelOff   Level = 32
)

// AppendLevel appends the short display string for a log level
// ("DBG", "INF", "WRN", "ERR") to buf. Unknown levels are appended
// as their numeric value.
func AppendLevel(buf []byte, level Level) []byte {
	switch level {
	case LevelDebug:
		return append(buf, "DBG"...)
	case LevelInfo:
		return append(buf, "INF"...)
	case LevelWarn:
		return append(buf, "WRN"...)
	case LevelError:
		return append(buf, "ERR"...)
	default:
		return strconv.AppendInt(buf, int64(level), 10)
	}
}

// Sink receives log entries for output. Implementations decide their
// own serialization format and manage their own scratch buffers
// internally. Flush forces any buffered data to be written (called
// before sleep).
type Sink interface {
	WriteLog(t time.Time, level Level, msg string) error
	WriteBytes(t time.Time, level Level, msg []byte) error
	Flush() error
}

// target pairs a Sink with the minimum level it should receive.
type target struct {
	sink     Sink
	minLevel Level
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
			if err := l.targets[i].sink.WriteLog(l.t, level, msg); err != nil {
				// Route to debug (UART) instead of logging through
				// ourselves to avoid a recursive log loop.
				debug.Log("sink error: " + err.Error())
			}
		}
	}
}

// Write logs b at the given level without converting b to a string.
func (l *Logger) Write(b []byte, level Level) {
	for i := range l.targets {
		if level >= l.targets[i].minLevel {
			if err := l.targets[i].sink.WriteBytes(l.t, level, b); err != nil {
				debug.Log("sink error: " + err.Error())
			}
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
