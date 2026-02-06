package hardware

import "time"

type Platform interface {
	Name() string
	ReadFile(name string) ([]byte, error)
	ReadTime() (time.Time, error)
	Sleep(d time.Duration) error
}
