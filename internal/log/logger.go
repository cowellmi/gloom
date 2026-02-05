package log

import (
	"time"
)

type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

type Logger struct {
	serialEnabled bool
}

func NewLogger(serialEnabled bool) *Logger {
	return &Logger{
		serialEnabled: serialEnabled,
	}
}

func (l *Logger) Log(t time.Time, level Level, msg string) {
	if l.serialEnabled {
		logToSerial(t, level, msg)
	}
}

func logToSerial(t time.Time, level Level, msg string) {
	print("[")
	printTwoDigits(t.Hour())
	print(":")
	printTwoDigits(t.Minute())
	print(":")
	printTwoDigits(t.Second())
	print("] ")

	printLevel(level)

	print(" | ")
	println(msg)
}

func printTwoDigits(n int) {
	if n < 10 {
		print("0")
	}
	print(n)
}

func printLevel(level Level) {
	switch level {
	case LevelDebug:
		print("DEBUG")

	case LevelInfo:
		print("INFO")

	case LevelWarn:
		print("WARNING")

	case LevelError:
		print("ERROR")

	default:
		print("INVALID_LEVEL")
	}
}
