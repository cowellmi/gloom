package rtc

import "time"

type AlarmID uint8

type Clock interface {
	ClearAlarm(id AlarmID) error
	IsAlarmFired(id AlarmID) bool
	ReadTime() (time.Time, error)
	SetAlarm(id AlarmID, duration time.Duration) error
}
