package hal

import (
	"testing"
	"time"
)

var T = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// --- mocks ---

type mockMCU struct {
	calls       []string
	armCalls    []uint8
	disarmCalls []uint8
}

func (m *mockMCU) Identifier() string    { return "mock-mcu" }
func (m *mockMCU) EnableWatchdog()       { m.calls = append(m.calls, "EnableWatchdog") }
func (m *mockMCU) DisableWatchdog()      { m.calls = append(m.calls, "DisableWatchdog") }
func (m *mockMCU) PetWatchdog()          { m.calls = append(m.calls, "PetWatchdog") }
func (m *mockMCU) RecoverI2C(_, _ uint8) {}
func (m *mockMCU) Standby()              { m.calls = append(m.calls, "Standby") }

func (m *mockMCU) ArmWake(pin uint8) error {
	m.calls = append(m.calls, "ArmWake")
	m.armCalls = append(m.armCalls, pin)
	return nil
}

func (m *mockMCU) DisarmWake(pin uint8) {
	m.calls = append(m.calls, "DisarmWake")
	m.disarmCalls = append(m.disarmCalls, pin)
}

// mockRTC returns pre-programmed times on successive ReadTime calls,
// simulating time advancing while the MCU is in standby.
type mockRTC struct {
	times      []time.Time
	timeIdx    int
	pin        uint8
	setWakes   []time.Time
	clearCount int
}

func (m *mockRTC) Identifier() string { return "mock-rtc" }

func (m *mockRTC) ReadTime() (time.Time, error) {
	if m.timeIdx >= len(m.times) {
		return m.times[len(m.times)-1], nil
	}
	t := m.times[m.timeIdx]
	m.timeIdx++
	return t, nil
}

func (m *mockRTC) SetWake(target time.Time) error {
	m.setWakes = append(m.setWakes, target)
	return nil
}

func (m *mockRTC) ClearWake() error {
	m.clearCount++
	return nil
}

func (m *mockRTC) WakePin() uint8 { return m.pin }

type mockRails struct {
	powerOnCalls  []WakeReason
	powerOffCount int
	d             time.Duration
}

func (m *mockRails) PowerOn(reason WakeReason) {
	m.powerOnCalls = append(m.powerOnCalls, reason)
}

func (m *mockRails) PowerOff()            { m.powerOffCount++ }
func (m *mockRails) Delay() time.Duration { return m.d }

// --- helpers ---

func callIndex(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

// callIndexAfter returns the first index of name at or after start.
func callIndexAfter(calls []string, name string, start int) int {
	for i := start; i < len(calls); i++ {
		if calls[i] == name {
			return i
		}
	}
	return -1
}

// --- tests ---

func TestEarliest(t *testing.T) {
	a := T
	b := T.Add(5 * time.Second)

	if got := earliest(a, b); !got.Equal(a) {
		t.Errorf("earliest(a, b) = %v, want %v", got, a)
	}
	if got := earliest(b, a); !got.Equal(a) {
		t.Errorf("earliest(b, a) = %v, want %v", got, a)
	}
	if got := earliest(time.Time{}, b); !got.Equal(b) {
		t.Errorf("earliest(zero, b) = %v, want %v", got, b)
	}
	if got := earliest(a, time.Time{}); !got.Equal(a) {
		t.Errorf("earliest(a, zero) = %v, want %v", got, a)
	}
	if got := earliest(time.Time{}, time.Time{}); !got.IsZero() {
		t.Errorf("earliest(zero, zero) = %v, want zero", got)
	}
}

func TestNewSystem_RTCWakePin(t *testing.T) {
	rtc := &mockRTC{pin: 12, times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	if len(sys.wakePins) != 1 || sys.wakePins[0] != 12 {
		t.Errorf("wakePins = %v, want [12]", sys.wakePins)
	}
}

func TestNewSystem_NilRTC(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil)
	if len(sys.wakePins) != 0 {
		t.Errorf("wakePins = %v, want empty", sys.wakePins)
	}
}

func TestAddWakePin(t *testing.T) {
	rtc := &mockRTC{pin: 12, times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil)
	sys.AddWakePin(7)

	if len(sys.wakePins) != 2 {
		t.Fatalf("wakePins = %v, want [12 7]", sys.wakePins)
	}
	if sys.wakePins[0] != 12 || sys.wakePins[1] != 7 {
		t.Errorf("wakePins = %v, want [12 7]", sys.wakePins)
	}
}

func TestReadTime_WithRTC(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	got, err := sys.ReadTime()
	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if !got.Equal(T) {
		t.Errorf("ReadTime() = %v, want %v", got, T)
	}
}

func TestReadTime_WithoutRTC(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil)

	before := time.Now()
	got, err := sys.ReadTime()
	after := time.Now()

	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("ReadTime() = %v, not between %v and %v", got, before, after)
	}
}

func TestSleep_SampleWake(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, &mockRails{})

	reason, err := sys.Sleep(10*time.Second, 0)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeSample {
		t.Errorf("reason = %d, want WakeSample (%d)", reason, WakeSample)
	}
}

func TestSleep_HeartbeatWake(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(61 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	reason, err := sys.Sleep(0, time.Minute)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeHeartbeat {
		t.Errorf("reason = %d, want WakeHeartbeat (%d)", reason, WakeHeartbeat)
	}
}

func TestSleep_SampleBeforeHeartbeat(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	reason, err := sys.Sleep(10*time.Second, time.Minute)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeSample {
		t.Errorf("reason = %d, want WakeSample", reason)
	}
}

func TestSleep_HeartbeatBeforeSample(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(6 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	reason, err := sys.Sleep(time.Minute, 5*time.Second)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeHeartbeat {
		t.Errorf("reason = %d, want WakeHeartbeat", reason)
	}
}

func TestSleep_ExternalWake(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(time.Second)},
		pin:   12,
	}
	rails := &mockRails{}
	sys := NewSystem(&mockMCU{}, rtc, rails)

	reason, err := sys.Sleep(0, 0)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeExternal {
		t.Errorf("reason = %d, want WakeExternal (%d)", reason, WakeExternal)
	}
	// External wake must not trigger reason-specific PowerOn.
	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != WakeAlways {
		t.Errorf("PowerOn calls = %v, want [WakeAlways]", rails.powerOnCalls)
	}
}

func TestSleep_RailDelaySubtracted(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(10 * time.Second)},
		pin:   12,
	}
	rails := &mockRails{d: time.Millisecond}
	sys := NewSystem(&mockMCU{}, rtc, rails)

	reason, err := sys.Sleep(10*time.Second, 0)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if reason != WakeSample {
		t.Fatalf("reason = %d, want WakeSample", reason)
	}
	// Alarm target should be T + (10s - 1ms), not T + 10s.
	if len(rtc.setWakes) != 1 {
		t.Fatalf("SetWake called %d times, want 1", len(rtc.setWakes))
	}
	want := T.Add(10*time.Second - time.Millisecond)
	if !rtc.setWakes[0].Equal(want) {
		t.Errorf("SetWake target = %v, want %v", rtc.setWakes[0], want)
	}
}

func TestSleep_RailSequencing(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	rails := &mockRails{}
	sys := NewSystem(&mockMCU{}, rtc, rails)

	reason, _ := sys.Sleep(10*time.Second, 0)
	if reason != WakeSample {
		t.Fatalf("reason = %d, want WakeSample", reason)
	}

	if rails.powerOffCount != 1 {
		t.Errorf("PowerOff called %d times, want 1", rails.powerOffCount)
	}
	// PowerOn: first WakeAlways (core rails after wake), then
	// WakeSample (reason-specific rails for sensors).
	if len(rails.powerOnCalls) != 2 {
		t.Fatalf("PowerOn called %d times, want 2; got %v", len(rails.powerOnCalls), rails.powerOnCalls)
	}
	if rails.powerOnCalls[0] != WakeAlways {
		t.Errorf("first PowerOn = %d, want WakeAlways (%d)", rails.powerOnCalls[0], WakeAlways)
	}
	if rails.powerOnCalls[1] != WakeSample {
		t.Errorf("second PowerOn = %d, want WakeSample (%d)", rails.powerOnCalls[1], WakeSample)
	}
}

func TestSleep_NilRails(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	_, err := sys.Sleep(10*time.Second, 0)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
}

func TestSleep_DeadlineResets(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{
			T,                       // call 1: before sleep
			T.Add(11 * time.Second), // call 1: after wake
			T.Add(11 * time.Second), // call 2: before sleep
			T.Add(22 * time.Second), // call 2: after wake
		},
		pin: 12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil)

	reason, _ := sys.Sleep(10*time.Second, 0)
	if reason != WakeSample {
		t.Fatalf("first Sleep: reason = %d, want WakeSample", reason)
	}

	// After firing, nextSample was cleared. Second call sets a fresh
	// deadline from the new "now" (T+11s) → target T+21s.
	reason, _ = sys.Sleep(10*time.Second, 0)
	if reason != WakeSample {
		t.Fatalf("second Sleep: reason = %d, want WakeSample", reason)
	}

	if len(rtc.setWakes) != 2 {
		t.Fatalf("SetWake called %d times, want 2", len(rtc.setWakes))
	}
	want := T.Add(21 * time.Second)
	if !rtc.setWakes[1].Equal(want) {
		t.Errorf("second SetWake target = %v, want %v", rtc.setWakes[1], want)
	}
}

func TestSleep_DeepSleepSequence(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	mcu := &mockMCU{}
	rails := &mockRails{}
	sys := NewSystem(mcu, rtc, rails)
	sys.AddWakePin(7)

	sys.Sleep(10*time.Second, 0)

	standbyAt := callIndex(mcu.calls, "Standby")
	if standbyAt < 0 {
		t.Fatal("Standby was not called")
	}

	// ArmWake must precede Standby.
	armAt := callIndex(mcu.calls, "ArmWake")
	if armAt < 0 || armAt > standbyAt {
		t.Error("ArmWake must precede Standby")
	}

	// DisableWatchdog must precede Standby.
	disableAt := callIndex(mcu.calls, "DisableWatchdog")
	if disableAt < 0 || disableAt > standbyAt {
		t.Error("DisableWatchdog must precede Standby")
	}

	// DisarmWake and EnableWatchdog must follow Standby.
	disarmAt := callIndexAfter(mcu.calls, "DisarmWake", standbyAt)
	if disarmAt < 0 {
		t.Error("DisarmWake must follow Standby")
	}
	enableAt := callIndexAfter(mcu.calls, "EnableWatchdog", standbyAt)
	if enableAt < 0 {
		t.Error("EnableWatchdog must follow Standby")
	}

	// Both pins armed and disarmed (RTC pin 12 + external pin 7).
	if len(mcu.armCalls) != 2 || mcu.armCalls[0] != 12 || mcu.armCalls[1] != 7 {
		t.Errorf("ArmWake pins = %v, want [12 7]", mcu.armCalls)
	}
	if len(mcu.disarmCalls) != 2 || mcu.disarmCalls[0] != 12 || mcu.disarmCalls[1] != 7 {
		t.Errorf("DisarmWake pins = %v, want [12 7]", mcu.disarmCalls)
	}

	// RTC alarm lifecycle: ClearWake before SetWake, ClearWake after wake.
	if rtc.clearCount != 2 {
		t.Errorf("ClearWake called %d times, want 2", rtc.clearCount)
	}
	if len(rtc.setWakes) != 1 {
		t.Fatalf("SetWake called %d times, want 1", len(rtc.setWakes))
	}
	want := T.Add(10 * time.Second)
	if !rtc.setWakes[0].Equal(want) {
		t.Errorf("SetWake target = %v, want %v", rtc.setWakes[0], want)
	}
}
