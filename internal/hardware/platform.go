package hardware

import "time"

type Platform interface {
	Now() (time.Time, error)
	Sleep(d time.Duration)
	ReadFile(name string) ([]byte, error)
}
