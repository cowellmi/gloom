// Package log defines the Level type used throughout Gloom for log
// severity filtering. Runtime log output goes through sinks (see
// internal/sink); this package only provides the shared type.
package log

type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)
