package hal

import "time"

// Fallback is the degraded-mode Platform used when the primary board
// (e.g. Hypnos) fails on probe.
type Fallback struct {
	nextSample    time.Time
	nextHeartbeat time.Time
}

func (*Fallback) Identifier() string { return "Fallback" }

func (*Fallback) ReadTime() (time.Time, error) {
	return time.Now(), nil
}

// Sleep picks the earliest of the sample and heartbeat deadlines,
// sleeps until that time, and returns the corresponding wake reason.
// A zero interval means that deadline is disabled.
func (f *Fallback) Sleep(sampleInterval, heartbeatInterval time.Duration) (WakeReason, error) {
	now := time.Now()

	if sampleInterval > 0 && f.nextSample.IsZero() {
		f.nextSample = now.Add(sampleInterval)
	}
	if heartbeatInterval > 0 && f.nextHeartbeat.IsZero() {
		f.nextHeartbeat = now.Add(heartbeatInterval)
	}

	target := earliest(f.nextSample, f.nextHeartbeat)

	if !target.IsZero() && now.Before(target) {
		time.Sleep(target.Sub(now))
	}

	now = time.Now()

	if sampleInterval > 0 && !f.nextSample.IsZero() && !now.Before(f.nextSample) {
		f.nextSample = time.Time{}
		return WakeSample, nil
	}
	if heartbeatInterval > 0 && !f.nextHeartbeat.IsZero() && !now.Before(f.nextHeartbeat) {
		f.nextHeartbeat = time.Time{}
		return WakeHeartbeat, nil
	}

	return WakeExternal, nil
}

// earliest returns the earlier of two times. If either is zero
// (unscheduled), the other is returned. If both are zero, zero is
// returned.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.Before(b) {
		return a
	}
	return b
}
