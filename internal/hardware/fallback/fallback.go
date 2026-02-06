package fallback

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
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

func (*Platform) Sleep(sample, heartbeat time.Duration) (hardware.WakeReason, error) {
	// In fallback mode there is no RTC or deep sleep. Sleep for the
	// shorter of the two intervals and return the corresponding reason.
	d := sample
	reason := hardware.WakeSample

	if heartbeat > 0 && (sample == 0 || heartbeat < sample) {
		d = heartbeat
		reason = hardware.WakeHeartbeat
	}

	if d > 0 {
		time.Sleep(d)
	}

	return reason, nil
}
