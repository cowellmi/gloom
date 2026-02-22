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
	firedPins   map[uint8]bool
}

func (m *mockMCU) Identifier() string           { return "mock-mcu" }
func (m *mockMCU) EnableWatchdog()               { m.calls = append(m.calls, "EnableWatchdog") }
func (m *mockMCU) DisableWatchdog()              { m.calls = append(m.calls, "DisableWatchdog") }
func (m *mockMCU) PetWatchdog()                  { m.calls = append(m.calls, "PetWatchdog") }
func (m *mockMCU) ConfigureI2C(_, _ uint8) error { return nil }
func (m *mockMCU) Standby()                      { m.calls = append(m.calls, "Standby") }
func (m *mockMCU) PaintStack()                   {}
func (m *mockMCU) StackSize() uint               { return 0 }
func (m *mockMCU) StackFree() uint               { return 0 }

type mockLED struct{}

func (l *mockLED) Configure(_ uint8) {}
func (l *mockLED) On()               {}
func (l *mockLED) Off()              {}

func (m *mockMCU) ArmWake(pin uint8) error {
	m.calls = append(m.calls, "ArmWake")
	m.armCalls = append(m.armCalls, pin)
	return nil
}

func (m *mockMCU) DisarmWake(pin uint8) {
	m.calls = append(m.calls, "DisarmWake")
	m.disarmCalls = append(m.disarmCalls, pin)
}

func (m *mockMCU) PinFired(pin uint8) bool {
	return m.firedPins[pin]
}

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

func firedSlots(fired []bool) []int {
	var result []int
	for i, f := range fired {
		if f {
			result = append(result, i)
		}
	}
	return result
}

// --- tests ---

func TestEarliestDeadline(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}, pin: 12}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second, 5 * time.Second})

	// Force deadlines to be set.
	sys.deadlines[0] = T.Add(10 * time.Second)
	sys.deadlines[1] = T.Add(5 * time.Second)

	got, hasDeadlines := sys.earliestDeadline()
	want := T.Add(5 * time.Second)
	if !hasDeadlines {
		t.Error("earliestDeadline() hasDeadlines = false, want true")
	}
	if !got.Equal(want) {
		t.Errorf("earliestDeadline() = %v, want %v", got, want)
	}
}

func TestNewSystem_RTCWakePin(t *testing.T) {
	rtc := &mockRTC{pin: 12, times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil, nil)

	if len(sys.wakePins) != 1 || sys.wakePins[0] != 12 {
		t.Errorf("wakePins = %v, want [12]", sys.wakePins)
	}
}

func TestNewSystem_NilRTC(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil, nil)
	if len(sys.wakePins) != 0 {
		t.Errorf("wakePins = %v, want empty", sys.wakePins)
	}
}

func TestRegisterExternalPin(t *testing.T) {
	rtc := &mockRTC{pin: 12, times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{0, 0})
	sys.RegisterExternalPin(7, 1)

	if len(sys.wakePins) != 2 || sys.wakePins[1] != 7 {
		t.Errorf("wakePins = %v, want [12 7]", sys.wakePins)
	}
	if len(sys.extPins) != 1 || sys.extPins[0].pin != 7 || sys.extPins[0].slot != 1 {
		t.Errorf("extPins = %v, want [{7 1}]", sys.extPins)
	}
}

func TestRegisterExternalPin_NoDuplicate(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil, []time.Duration{0, 0})
	sys.RegisterExternalPin(7, 0)
	sys.RegisterExternalPin(7, 1)

	if len(sys.wakePins) != 1 {
		t.Errorf("wakePins = %v, want [7] (no duplicate)", sys.wakePins)
	}
	if len(sys.extPins) != 2 {
		t.Errorf("extPins = %v, want 2 entries (same pin, different slots)", sys.extPins)
	}
}

func TestReadTime_WithRTC(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}}
	sys := NewSystem(&mockMCU{}, rtc, nil, nil)

	got, err := sys.ReadTime()
	if err != nil {
		t.Fatalf("ReadTime() error: %v", err)
	}
	if !got.Equal(T) {
		t.Errorf("ReadTime() = %v, want %v", got, T)
	}
}

func TestReadTime_WithoutRTC(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil, nil)

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

func TestNextWake_FreshDeadlines(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}, pin: 12}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second, 3 * time.Second})

	d := sys.NextWake()
	if d != 3*time.Second {
		t.Errorf("NextWake() = %v, want 3s (nearest)", d)
	}
}

func TestNextWake_NoDeadlines(t *testing.T) {
	rtc := &mockRTC{times: []time.Time{T}, pin: 12}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{0, 0})

	d := sys.NextWake()
	if d != 0 {
		t.Errorf("NextWake() = %v, want 0", d)
	}
}

func TestNextWake_PartialRemaining(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{
			T,                      // Sleep: read time before sleep
			T.Add(4 * time.Second), // Sleep: read time after wake
			T.Add(4 * time.Second), // NextWake: read time
		},
		pin: 12,
	}
	// Slot 0: 10s interval, Slot 1: 3s interval
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second, 3 * time.Second})

	fired, _ := sys.Sleep()

	// Slot 1 (3s) should fire at T+3s when now is T+4s.
	if !fired[1] {
		t.Error("slot 1 should have fired")
	}
	if fired[0] {
		t.Error("slot 0 should not have fired")
	}

	d := sys.NextWake()
	// Slot 1 advanced from T+3s to T+6s. Remaining = 6-4 = 2s.
	// Slot 0 still at T+10s. Remaining = 10-4 = 6s.
	// Nearest is 2s.
	if d != 2*time.Second {
		t.Errorf("NextWake() = %v, want 2s", d)
	}
}

func TestSleep_SingleSlotFires(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, &mockRails{}, []time.Duration{10 * time.Second})

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] {
		t.Error("slot 0 should have fired")
	}
}

func TestSleep_MultiSlot_OnlyEarliestFires(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(6 * time.Second)},
		pin:   12,
	}
	// Slot 0: 1m interval, Slot 1: 5s interval
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{time.Minute, 5 * time.Second})

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if fired[0] {
		t.Error("slot 0 (1m) should not have fired")
	}
	if !fired[1] {
		t.Error("slot 1 (5s) should have fired")
	}
}

func TestSleep_SimultaneousFire(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	rails := &mockRails{}
	sys := NewSystem(&mockMCU{}, rtc, rails, []time.Duration{10 * time.Second, 10 * time.Second})

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] || !fired[1] {
		t.Errorf("fired = %v, want [true true]", fired)
	}

	// Both deadlines should advance.
	wantNext := T.Add(20 * time.Second)
	if !sys.deadlines[0].Equal(wantNext) {
		t.Errorf("deadlines[0] = %v, want %v", sys.deadlines[0], wantNext)
	}
	if !sys.deadlines[1].Equal(wantNext) {
		t.Errorf("deadlines[1] = %v, want %v", sys.deadlines[1], wantNext)
	}
}

func TestSleep_ExternalWake(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(time.Second)},
		pin:   12,
	}
	rails := &mockRails{}
	mcu := &mockMCU{firedPins: map[uint8]bool{7: true}}
	// No interval-based deadlines, one external pin on slot 0.
	sys := NewSystem(mcu, rtc, rails, []time.Duration{0})
	sys.RegisterExternalPin(7, 0)

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] {
		t.Error("external pin slot should have fired")
	}

	// Always-rails only — no sensor rail call.
	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != false {
		t.Errorf("PowerOn calls = %v, want [false]", rails.powerOnCalls)
	}
}

func TestSleep_ExternalWakeMultipleSlots(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(time.Second)},
		pin:   12,
	}
	mcu := &mockMCU{firedPins: map[uint8]bool{7: true, 8: true}}
	sys := NewSystem(mcu, rtc, nil, []time.Duration{0, 0, 0})
	sys.RegisterExternalPin(7, 0)
	sys.RegisterExternalPin(8, 2)

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] || fired[1] || !fired[2] {
		t.Errorf("fired = %v, want [true false true]", fired)
	}
}

func TestSleep_ExternalWakeSelectivePin(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(time.Second)},
		pin:   12,
	}
	// Only pin 7 fired; pin 8 did not.
	mcu := &mockMCU{firedPins: map[uint8]bool{7: true}}
	sys := NewSystem(mcu, rtc, nil, []time.Duration{0, 0, 0})
	sys.RegisterExternalPin(7, 0)
	sys.RegisterExternalPin(8, 2)

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] || fired[1] || fired[2] {
		t.Errorf("fired = %v, want [true false false]", fired)
	}
}

func TestSleep_DeadlineWithoutExternalPin(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	// Slot 0: timer-based, Slot 1: external only. Pin 7 did not fire.
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second, 0})
	sys.RegisterExternalPin(7, 1)

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] {
		t.Error("slot 0 (timer) should have fired")
	}
	if fired[1] {
		t.Error("slot 1 (external) should not fire when pin didn't trigger")
	}
}

func TestSleep_DeadlineAndExternalSimultaneous(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	// Both deadline and external pin fire during the same cycle.
	mcu := &mockMCU{firedPins: map[uint8]bool{7: true}}
	sys := NewSystem(mcu, rtc, nil, []time.Duration{10 * time.Second, 0})
	sys.RegisterExternalPin(7, 1)

	fired, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if !fired[0] {
		t.Error("slot 0 (timer) should have fired")
	}
	if !fired[1] {
		t.Error("slot 1 (external) should fire when pin triggered")
	}
}

func TestSleep_RailSequencing(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	rails := &mockRails{}
	sys := NewSystem(&mockMCU{}, rtc, rails, []time.Duration{10 * time.Second})

	fired, _ := sys.Sleep()
	if !fired[0] {
		t.Fatal("slot 0 should have fired")
	}

	if rails.powerOffCount != 1 {
		t.Errorf("PowerOff called %d times, want 1", rails.powerOffCount)
	}
	// Always-rails restored after wake.
	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != false {
		t.Errorf("PowerOn calls = %v, want [false]", rails.powerOnCalls)
	}
}

func TestPowerOnSensorRails(t *testing.T) {
	rails := &mockRails{}
	sys := NewSystem(&mockMCU{}, nil, rails, nil)

	sys.PowerOnSensorRails()

	if len(rails.powerOnCalls) != 1 || rails.powerOnCalls[0] != true {
		t.Errorf("PowerOn calls = %v, want [true]", rails.powerOnCalls)
	}
}

func TestPowerOnSensorRails_NilRails(t *testing.T) {
	sys := NewSystem(&mockMCU{}, nil, nil, nil)
	sys.PowerOnSensorRails() // should not panic
}

func TestSleep_NilRails(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{T, T.Add(11 * time.Second)},
		pin:   12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second})

	_, err := sys.Sleep()
	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
}

func TestSleep_DeadlineAdvances(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{
			T,                       // call 1: before sleep
			T.Add(11 * time.Second), // call 1: after wake
			T.Add(11 * time.Second), // call 2: before sleep
			T.Add(22 * time.Second), // call 2: after wake
		},
		pin: 12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second})

	fired, _ := sys.Sleep()
	if !fired[0] {
		t.Fatal("first Sleep: slot 0 should have fired")
	}

	fired, _ = sys.Sleep()
	if !fired[0] {
		t.Fatal("second Sleep: slot 0 should have fired")
	}

	// After first fire: T+10s → T+20s.
	// After second fire: T+20s → T+30s.
	if len(rtc.setWakes) != 2 {
		t.Fatalf("SetWake called %d times, want 2", len(rtc.setWakes))
	}
	want := T.Add(20 * time.Second)
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
	sys := NewSystem(mcu, rtc, rails, []time.Duration{10 * time.Second})
	sys.AddWakePin(7)

	sys.Sleep()

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

func TestSleep_FiredSliceReused(t *testing.T) {
	rtc := &mockRTC{
		times: []time.Time{
			T,                       // call 1
			T.Add(11 * time.Second), // call 1
			T.Add(11 * time.Second), // call 2
			T.Add(22 * time.Second), // call 2
		},
		pin: 12,
	}
	sys := NewSystem(&mockMCU{}, rtc, nil, []time.Duration{10 * time.Second, time.Minute})

	fired1, _ := sys.Sleep()
	if !fired1[0] || fired1[1] {
		t.Fatalf("first: fired = %v, want [true false]", fired1)
	}

	fired2, _ := sys.Sleep()
	if !fired2[0] || fired2[1] {
		t.Fatalf("second: fired = %v, want [true false]", fired2)
	}

	// Verify it's the same backing slice (reused).
	if &fired1[0] != &fired2[0] {
		t.Error("fired slices should be the same backing array (reused)")
	}
}
