package sleeper

import (
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
}

func (m *mockMCU) Identifier() string              { return "mock-mcu" }
func (m *mockMCU) EnableWatchdog()                 { m.calls = append(m.calls, "EnableWatchdog") }
func (m *mockMCU) DisableWatchdog()                { m.calls = append(m.calls, "DisableWatchdog") }
func (m *mockMCU) PetWatchdog()                    { m.calls = append(m.calls, "PetWatchdog") }
func (m *mockMCU) ConfigureI2C(_, _ hal.Pin) error { return nil }
func (m *mockMCU) Standby()                        { m.calls = append(m.calls, "Standby") }
func (m *mockMCU) PaintStack()                     {}
func (m *mockMCU) StackSize() uint                 { return 0 }
func (m *mockMCU) StackFree() uint                 { return 0 }

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

type mockRTC struct {
	times      []time.Time
	timeIdx    int
	pin        hal.Pin
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

func (m *mockRTC) WakePin() hal.Pin { return m.pin }

type mockRails struct {
	powerOnCalls  []bool
	powerOffCount int
}

func (m *mockRails) PowerOn(sensors bool) {
	m.powerOnCalls = append(m.powerOnCalls, sensors)
}

func (m *mockRails) PowerOff() { m.powerOffCount++ }

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

func TestNewSleeper_RTCWakePin(t *testing.T) {
	rtc := &mockRTC{pin: 12, times: []time.Time{T}}
	s := New(&mockMCU{}, rtc, nil)

	if len(s.wakePins) != 1 || s.wakePins[0] != 12 {
		t.Errorf("wakePins = %v, want [12]", s.wakePins)
	}
	if s.hasExtPins {
		t.Error("hasExtPins should be false after New (RTC pin is not an ext interrupt)")
	}
}

func TestNewSleeper_NilRTC(t *testing.T) {
	s := New(&mockMCU{}, nil, nil)
	if len(s.wakePins) != 0 {
		t.Errorf("wakePins = %v, want empty", s.wakePins)
	}
	if s.hasExtPins {
		t.Error("hasExtPins should be false")
	}
}

// --- AddWakePin tests ---

func TestAddWakePin(t *testing.T) {
	s := New(&mockMCU{}, nil, nil)
	s.AddWakePin(7)

	if len(s.wakePins) != 1 || s.wakePins[0] != 7 {
		t.Errorf("wakePins = %v, want [7]", s.wakePins)
	}
	if !s.hasExtPins {
		t.Error("hasExtPins should be true after AddWakePin")
	}
}

func TestAddWakePin_NoDuplicate(t *testing.T) {
	s := New(&mockMCU{}, nil, nil)
	s.AddWakePin(7)
	s.AddWakePin(7)

	if len(s.wakePins) != 1 {
		t.Errorf("wakePins = %v, want [7] (no duplicate)", s.wakePins)
	}
	if !s.hasExtPins {
		t.Error("hasExtPins should be true")
	}
}

// --- ReadTime tests ---

func TestReadTime_WithRTC(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}}
	s := New(&mockMCU{}, rtc, nil)

	got, err := s.ReadTime()
	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if !got.Equal(T) {
		t.Errorf("ReadTime() = %v, want %v", got, T)
	}
}

func TestReadTime_WithoutRTC(t *testing.T) {
	s := New(&mockMCU{}, nil, nil)

	before := time.Now()
	got, err := s.ReadTime()
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
		pin:   12,
	}
	rails := &mockRails{}
	target := T.Add(10 * time.Second)
	s := New(&mockMCU{}, rtc, rails)

	_, err := s.Sleep(target)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}

	if rails.powerOffCount != 1 {
		t.Errorf("PowerOff called %d times, want 1", rails.powerOffCount)
	}
	// Always-rails restored after wake.
	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != false {
		t.Errorf("PowerOn calls = %v, want [false]", rails.powerOnCalls)
	}
}

func TestSleep_NilRails(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	target := T.Add(10 * time.Second)
	s := New(&mockMCU{}, rtc, nil)

	_, err := s.Sleep(target)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
}

func TestSleep_DeepSleepSequence(t *testing.T) {
	rtc := &mockRTC{
		// First call: before-sleep guard. Second call: after-sleep wake time.
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	mcu := &mockMCU{}
	rails := &mockRails{}
	target := T.Add(10 * time.Second)
	s := New(mcu, rtc, rails)
	s.AddWakePin(7)

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

	// RTC alarm lifecycle: ClearWake before SetWake, ClearWake after wake.
	if rtc.clearCount != 2 {
		t.Errorf("ClearWake called %d times, want 2", rtc.clearCount)
	}
}

func TestSleep_ZeroTarget_HasExtPins_DeepSleeps(t *testing.T) {
	mcu := &mockMCU{}
	// Use nil RTC so no RTC alarm is set; deep sleep driven by ext pin only.
	s := New(mcu, nil, nil)
	s.AddWakePin(7) // sets hasExtPins = true

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
		pin:   12,
	}
	// 10ms remaining — less than minDeepSleep (2s) → idle sleep, not deep sleep.
	target := T.Add(10 * time.Millisecond)
	s := New(mcu, rtc, nil)

	s.Sleep(target)

	if callIndex(mcu.calls, "Standby") >= 0 {
		t.Error("Standby should not be called when remaining < minDeepSleep")
	}
}

// --- PowerOnSensorRails tests ---

func TestPowerOnSensorRails(t *testing.T) {
	rails := &mockRails{}
	s := New(&mockMCU{}, nil, rails)

	s.PowerOnSensorRails()

	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != true {
		t.Errorf("PowerOn calls = %v, want [true]", rails.powerOnCalls)
	}
}

func TestPowerOnSensorRails_NilRails(t *testing.T) {
	s := New(&mockMCU{}, nil, nil)
	s.PowerOnSensorRails() // should not panic
}

// --- Sleep return value test ---

func TestSleep_ReturnsWakeTime(t *testing.T) {
	wakeTime := T.Add(11 * time.Second)
	rtc := &mockRTC{
		// First call: before-sleep guard (T). Second call: actual wake time.
		times: []time.Time{T, wakeTime},
		pin:   12,
	}
	target := T.Add(10 * time.Second)
	s := New(&mockMCU{}, rtc, nil)

	got, err := s.Sleep(target)
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !got.Equal(wakeTime) {
		t.Errorf("Sleep() returned %v, want %v", got, wakeTime)
	}
}
