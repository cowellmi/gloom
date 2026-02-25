package sleeper

import (
	"errors"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
)

var T = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// --- mocks ---

type mockMCU struct {
	calls       []string
	armCalls    []hal.Pin
	disarmCalls []hal.Pin
	firedPins   map[hal.Pin]bool
	activePins  map[hal.Pin]bool
}

func (m *mockMCU) Identifier() string              { return "mock-mcu" }
func (m *mockMCU) EnableWatchdog()                 { m.calls = append(m.calls, "EnableWatchdog") }
func (m *mockMCU) DisableWatchdog()                { m.calls = append(m.calls, "DisableWatchdog") }
func (m *mockMCU) PetWatchdog()                    { m.calls = append(m.calls, "PetWatchdog") }
func (m *mockMCU) ConfigureI2C(_, _ hal.Pin) error { return nil }
func (m *mockMCU) Standby()                        { m.calls = append(m.calls, "Standby") }
func (m *mockMCU) PaintStack()                     {}
func (m *mockMCU) StackSize() uint                 { return 0 }
func (m *mockMCU) StackUsed() uint                 { return 0 }

func (m *mockMCU) ArmWake(pin hal.Pin) error {
	m.calls = append(m.calls, "ArmWake")
	m.armCalls = append(m.armCalls, pin)
	return nil
}

func (m *mockMCU) DisarmWake(pin hal.Pin) {
	m.calls = append(m.calls, "DisarmWake")
	m.disarmCalls = append(m.disarmCalls, pin)
}

func (m *mockMCU) PinFired(pin hal.Pin) bool {
	return m.firedPins[pin]
}

func (m *mockMCU) PinActive(pin hal.Pin) bool {
	return m.activePins[pin]
}

type mockRTC struct {
	times      []time.Time
	timeIdx    int
	setWakes   []time.Time
	clearCount int
	noAlarm    bool // if true, HasAlarm() returns false
}

func (m *mockRTC) Identifier() string { return "mock-rtc" }
func (m *mockRTC) HasAlarm() bool     { return !m.noAlarm }

func (m *mockRTC) ReadTime() (time.Time, error) {
	if m.timeIdx >= len(m.times) {
		return m.times[len(m.times)-1], nil
	}
	t := m.times[m.timeIdx]
	m.timeIdx++
	return t, nil
}

func (m *mockRTC) SetAlarm(target time.Time) error {
	m.setWakes = append(m.setWakes, target)
	return nil
}

func (m *mockRTC) ClearAlarm() error {
	m.clearCount++
	return nil
}

type mockRails struct {
	powerCalls []hal.RailState
}

func (m *mockRails) Power(state hal.RailState) {
	m.powerCalls = append(m.powerCalls, state)
}

// mockMCUWithArmError wraps mockMCU and returns an error from ArmWake
// on the call at index failAt (0-based). All other methods are promoted
// from the embedded mockMCU so the interface is fully satisfied.
type mockMCUWithArmError struct {
	mockMCU
	failAt   int
	armCount int
}

func (m *mockMCUWithArmError) ArmWake(pin hal.Pin) error {
	m.calls = append(m.calls, "ArmWake")
	m.armCalls = append(m.armCalls, pin)
	idx := m.armCount
	m.armCount++
	if idx == m.failAt {
		return errors.New("mock ArmWake failure")
	}
	return nil
}

// --- helpers ---

func callIndex(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

func callIndexAfter(calls []string, name string, start int) int {
	for i := start; i < len(calls); i++ {
		if calls[i] == name {
			return i
		}
	}
	return -1
}

// --- constructor tests ---

func TestNewSleeper_NilRTC(t *testing.T) {
	s := New(&mockMCU{}, nil, nil, nil)
	if len(s.wakePins) != 0 {
		t.Errorf("wakePins = %v, want empty", s.wakePins)
	}
}

// --- ReadTime tests ---

func TestReadTime_WithRTC(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}}
	s := New(&mockMCU{}, rtc, nil, nil)

	got, err := s.readTime()
	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if !got.Equal(T) {
		t.Errorf("ReadTime() = %v, want %v", got, T)
	}
}

func TestReadTime_WithoutRTC(t *testing.T) {
	s := New(&mockMCU{}, nil, nil, nil)

	before := time.Now()
	got, err := s.readTime()
	after := time.Now()

	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("ReadTime() = %v, not between %v and %v", got, before, after)
	}
}

// --- Sleep hardware behavior tests ---

func TestSleep_RailSequencing(t *testing.T) {
	rtc := &mockRTC{
		// First call: before-sleep guard (returns T so remaining=10s>0).
		// Second call: after-sleep wake time.
		times: []time.Time{T, T.Add(11 * time.Second)},
	}
	rails := &mockRails{}
	target := T.Add(10 * time.Second)
	s := New(&mockMCU{}, rtc, rails, []hal.Pin{12})

	_, err := s.Sleep(target)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}

	// Expect RailsOff before deep sleep, then RailsCore after wake.
	wantCalls := []hal.RailState{hal.RailsOff, hal.RailsCore}
	if len(rails.powerCalls) != 2 || rails.powerCalls[0] != hal.RailsOff || rails.powerCalls[1] != hal.RailsCore {
		t.Errorf("Power calls = %v, want %v", rails.powerCalls, wantCalls)
	}
}


func TestSleep_DeepSleepSequence(t *testing.T) {
	rtc := &mockRTC{
		// First call: before-sleep guard. Second call: after-sleep wake time.
		times: []time.Time{T, T.Add(11 * time.Second)},
	}
	mcu := &mockMCU{}
	rails := &mockRails{}
	target := T.Add(10 * time.Second)
	s := New(mcu, rtc, rails, []hal.Pin{12, 7})

	s.Sleep(target)

	standbyAt := callIndex(mcu.calls, "Standby")
	if standbyAt < 0 {
		t.Fatal("Standby was not called")
	}

	armAt := callIndex(mcu.calls, "ArmWake")
	if armAt < 0 || armAt > standbyAt {
		t.Error("ArmWake must precede Standby")
	}

	disableAt := callIndex(mcu.calls, "DisableWatchdog")
	if disableAt < 0 || disableAt > standbyAt {
		t.Error("DisableWatchdog must precede Standby")
	}

	disarmAt := callIndexAfter(mcu.calls, "DisarmWake", standbyAt)
	if disarmAt < 0 {
		t.Error("DisarmWake must follow Standby")
	}
	enableAt := callIndexAfter(mcu.calls, "EnableWatchdog", standbyAt)
	if enableAt < 0 {
		t.Error("EnableWatchdog must follow Standby")
	}

	// Both pins armed (RTC pin 12 + extra pin 7).
	if len(mcu.armCalls) != 2 || mcu.armCalls[0] != 12 || mcu.armCalls[1] != 7 {
		t.Errorf("ArmWake pins = %v, want [12 7]", mcu.armCalls)
	}

	// RTC alarm lifecycle: ClearAlarm before SetAlarm, ClearAlarm after wake.
	if rtc.clearCount != 2 {
		t.Errorf("ClearAlarm called %d times, want 2", rtc.clearCount)
	}
}

func TestSleep_ZeroTarget_HasExtPins_DeepSleeps(t *testing.T) {
	mcu := &mockMCU{}
	// Use nil RTC so no RTC alarm is set; deep sleep driven by ext pin only.
	s := New(mcu, nil, &mockRails{}, []hal.Pin{7})

	// Sleep with zero target: external-interrupt-only deep sleep.
	// In the test the mock Standby() returns immediately.
	s.Sleep(time.Time{})

	if callIndex(mcu.calls, "Standby") < 0 {
		t.Error("Standby should be called for zero target with ext pins")
	}
}

func TestSleep_InsufficientRemaining_NoDeepSleep(t *testing.T) {
	mcu := &mockMCU{}
	rtc := &mockRTC{
		// First call: T (before-sleep guard), so remaining = 10ms < minDeepSleep.
		// Second call: after-sleep wake time.
		times: []time.Time{T, T.Add(10 * time.Millisecond)},
	}
	// 10ms remaining — less than minDeepSleep (2s) → idle sleep, not deep sleep.
	target := T.Add(10 * time.Millisecond)
	s := New(mcu, rtc, &mockRails{}, []hal.Pin{12})

	s.Sleep(target)

	if callIndex(mcu.calls, "Standby") >= 0 {
		t.Error("Standby should not be called when remaining < minDeepSleep")
	}
}

func TestSleep_DeepSleepFallbackToIdle(t *testing.T) {
	// failAt:1 — first ArmWake (RTC pin 12) succeeds; second (ext pin 7) fails.
	// deepSleep rolls back by disarming pin 12, then returns the error.
	// Sleep must fall back to idleSleep and ultimately restore RailsCore.
	mcu := &mockMCUWithArmError{failAt: 1}
	rtc := &mockRTC{
		// Call 1: Sleep guard (T, remaining=10s → tries deep sleep).
		// Call 2: idleSleep initial read (T+11s ≥ target, loop exits immediately).
		// Call 3: final wake-time read (mock clamps to last value).
		times: []time.Time{T, T.Add(11 * time.Second)},
	}
	rails := &mockRails{}
	s := New(mcu, rtc, rails, []hal.Pin{12, 7})

	_, err := s.Sleep(T.Add(10 * time.Second))
	if err != nil {
		t.Fatalf("Sleep() returned error: %v", err)
	}

	// Standby must NOT be called — deep sleep was aborted before entering Standby.
	if callIndex(mcu.calls, "Standby") >= 0 {
		t.Error("Standby should not be called after ArmWake failure")
	}

	// Rails must be restored to RailsCore as the last Power call.
	if len(rails.powerCalls) == 0 {
		t.Fatal("no Power calls recorded")
	}
	if last := rails.powerCalls[len(rails.powerCalls)-1]; last != hal.RailsCore {
		t.Errorf("last Power call = %v, want RailsCore", last)
	}

	// Rollback must disarm exactly the pins that were successfully armed before
	// the failure. Pin 12 (index 0) was armed; pin 7 (index 1) was not.
	// This verifies the j < i bound in the rollback loop is correct.
	if len(mcu.disarmCalls) != 1 {
		t.Fatalf("disarmCalls = %v, want [12] (rollback of first armed pin only)", mcu.disarmCalls)
	}
	if mcu.disarmCalls[0] != 12 {
		t.Errorf("disarmCalls[0] = %v, want 12", mcu.disarmCalls[0])
	}
}

// --- HasAlarm tests ---

func TestSleep_NoAlarmRTC_SkipsDeepSleep(t *testing.T) {
	rtc := &mockRTC{
		noAlarm: true,
		times:   []time.Time{T, T.Add(11 * time.Second)},
	}
	mcu := &mockMCU{}
	s := New(mcu, rtc, &mockRails{}, []hal.Pin{12})

	s.Sleep(T.Add(10 * time.Second))

	if callIndex(mcu.calls, "Standby") >= 0 {
		t.Error("Standby should not be called when RTC has no alarm support")
	}
	if len(rtc.setWakes) > 0 {
		t.Error("SetAlarm should not be called when HasAlarm is false")
	}
}

// --- Sleep return value test ---

func TestSleep_ReturnsWakeTime(t *testing.T) {
	wakeTime := T.Add(11 * time.Second)
	rtc := &mockRTC{
		// First call: before-sleep guard (T). Second call: actual wake time.
		times: []time.Time{T, wakeTime},
	}
	target := T.Add(10 * time.Second)
	s := New(&mockMCU{}, rtc, &mockRails{}, []hal.Pin{12})

	got, err := s.Sleep(target)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !got.Equal(wakeTime) {
		t.Errorf("Sleep() returned %v, want %v", got, wakeTime)
	}
}
