package fallback

import (
	"errors"
	"time"
)

type Platform struct{}

func New() *Platform {
	return &Platform{}
}

func (*Platform) Name() string { return "Fallback" }

func (*Platform) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("fallback: no storage available")
}

func (*Platform) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

func (*Platform) Sleep(d time.Duration) error {
	time.Sleep(d)

	return nil
}
