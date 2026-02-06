package fallback

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
)

type Fallback struct{}

func New() *Fallback {
	return &Fallback{}
}

func (*Fallback) Name() string { return "Fallback" }

func (*Fallback) ReadFile(string) ([]byte, error) {
	return nil, errors.New("fallback: no storage available")
}

func (*Fallback) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

func (*Fallback) Sleep(sampleInterval, _ time.Duration) (hardware.WakeReason, error) {
	time.Sleep(sampleInterval)

	return hardware.WakeSample, nil
}
