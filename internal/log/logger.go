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
	minLevel      Level
	serialEnabled bool
}

func NewLogger(minLevel Level, serialEnabled bool) *Logger {
	return &Logger{
		minLevel:      minLevel,
		serialEnabled: serialEnabled,
	}
}

func (l *Logger) Log(t time.Time, level Level, msg string) {
	if level < l.minLevel {
		return
	}

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
		print("DBG")

	case LevelInfo:
		print("INF")

	case LevelWarn:
		print("WRN")

	case LevelError:
		print("ERR")

	default:
		print("???")
	}
}
