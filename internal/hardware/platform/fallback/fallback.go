package fallback

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/hardware/platform"
)

type System struct{}

func New() *System {
	return &System{}
}

func (*System) Name() string { return "Fallback" }

func (*System) ReadFile(string) ([]byte, error) {
	return nil, errors.New("fallback: no storage available")
}

func (*System) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

func (*System) Sleep(sampleInterval, _ time.Duration) (platform.WakeReason, error) {
	time.Sleep(sampleInterval)
	return platform.WakeSample, nil
}
