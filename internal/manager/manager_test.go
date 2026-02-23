package manager

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

var T = time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

// --- mocks ---

type mockOutput struct {
	name        string
	readings    []sensor.Reading
	measIDs     []string
	logEntries  []string
	flushCalled bool
}

func (m *mockOutput) Name() string { return m.name }

func (m *mockOutput) Record(_ time.Time, id string, readings []sensor.Reading) error {
	m.measIDs = append(m.measIDs, id)
	m.readings = append(m.readings, readings...)
	return nil
}

func (m *mockOutput) WriteLog(_ time.Time, _ log.Level, msg string) error {
	m.logEntries = append(m.logEntries, msg)
	return nil
}

func (m *mockOutput) WriteBytes(_ time.Time, _ log.Level, msg []byte) error {
	m.logEntries = append(m.logEntries, string(msg))
	return nil
}

func (m *mockOutput) Flush() error {
	m.flushCalled = true
	return nil
}

func (m *mockOutput) hasLog(substr string) bool {
	for _, e := range m.logEntries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func (m *mockOutput) countLog(substr string) int {
	n := 0
	for _, e := range m.logEntries {
		if strings.Contains(e, substr) {
			n++
		}
	}
	return n
}

type mockSystem struct {
	sleepFn                 func(target time.Time) (time.Time, error)
	firedPins               map[hal.Pin]bool
	powerOnSensorRailsCalls int
}

func (m *mockSystem) Sleep(target time.Time) (time.Time, error) {
	return m.sleepFn(target)
}

func (m *mockSystem) PinFired(pin hal.Pin) bool {
	return m.firedPins[pin]
}

func (m *mockSystem) PowerOnSensorRails() {
	m.powerOnSensorRailsCalls++
}

type mockLED struct {
	onCalled  bool
	offCalled bool
}

func (l *mockLED) On()    { l.onCalled = true }
func (l *mockLED) Off()   { l.offCalled = true }
func (l *mockLED) Blink() {}

type mockSensor struct {
	id           string
	readings     []sensor.Reading
	measureErr   error
	measureCalls int
}

func (m *mockSensor) ID() string { return m.id }

func (m *mockSensor) Measure() ([]sensor.Reading, error) {
	m.measureCalls++
	return m.readings, m.measureErr
}

// --- helpers ---

// afterDeadlineSleep returns a sleepFn that ignores target and
// returns wakeTime (simulating the MCU waking after the deadline).
func afterDeadlineSleep(wakeTime time.Time) func(time.Time) (time.Time, error) {
	return func(_ time.Time) (time.Time, error) { return wakeTime, nil }
}

func newTestManager(sys *mockSystem, groups []Group, recorders []sensor.Recorder) (*Manager, *mockOutput) {
	mo := &mockOutput{name: "test"}

	logger := log.NewLogger(time.Time{})
	logger.AddSink(mo, log.LevelDebug)

	man := New(sys, groups, recorders, logger)
	man.wakeTime = T // seed initial time, as boot() would via ReadTime
	return man, mo
}

// --- deadline tracking tests (migrated from hal_test.go) ---

func TestEarliestDeadline(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{
		{Name: "a", Interval: 10 * time.Second},
		{Name: "b", Interval: 5 * time.Second},
	}
	man, _ := newTestManager(sys, groups, nil)

	// Initialize deadlines by running doSleep once up to the point where
	// earliestDeadline is meaningful; set them manually for precision.
	man.deadlines[0] = T.Add(10 * time.Second)
	man.deadlines[1] = T.Add(5 * time.Second)

	got := man.earliestDeadline()
	want := T.Add(5 * time.Second)
	if !got.Equal(want) {
		t.Errorf("earliestDeadline() = %v, want %v", got, want)
	}
}

func TestStep_SingleSlotFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{{Name: "a", Interval: 10 * time.Second}}
	man, _ := newTestManager(sys, groups, nil)
	man.doSleep()

	if !man.fired[0] {
		t.Error("slot 0 should have fired")
	}
}

func TestStep_MultiSlot_OnlyEarliestFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}
	groups := []Group{
		{Name: "slow", Interval: time.Minute},
		{Name: "fast", Interval: 5 * time.Second},
	}
	man, _ := newTestManager(sys, groups, nil)
	man.doSleep()

	if man.fired[0] {
		t.Error("slot 0 (1m) should not have fired")
	}
	if !man.fired[1] {
		t.Error("slot 1 (5s) should have fired")
	}
}

func TestStep_SimultaneousFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{
		{Name: "a", Interval: 10 * time.Second},
		{Name: "b", Interval: 10 * time.Second},
	}
	man, _ := newTestManager(sys, groups, nil)
	man.doSleep()

	if !man.fired[0] || !man.fired[1] {
		t.Errorf("fired = %v, want [true true]", man.fired)
	}

	// Both deadlines should have advanced.
	wantNext := T.Add(20 * time.Second)
	if !man.deadlines[0].Equal(wantNext) {
		t.Errorf("deadlines[0] = %v, want %v", man.deadlines[0], wantNext)
	}
	if !man.deadlines[1].Equal(wantNext) {
		t.Errorf("deadlines[1] = %v, want %v", man.deadlines[1], wantNext)
	}
}

func TestStep_DeadlineAdvances(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{{Name: "a", Interval: 10 * time.Second}}
	man, _ := newTestManager(sys, groups, nil)

	man.doSleep()
	if !man.fired[0] {
		t.Fatal("first doSleep: slot 0 should have fired")
	}
	// Deadline advances from T+10s to T+20s.
	want := T.Add(20 * time.Second)
	if !man.deadlines[0].Equal(want) {
		t.Errorf("deadline after first fire = %v, want %v", man.deadlines[0], want)
	}
}

func TestStep_ExternalPinFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	// Group with no interval — fires only via ext pin.
	groups := []Group{{Name: "ext", Interval: 0}}
	man, _ := newTestManager(sys, groups, nil)
	man.RegisterExternalPin(7, 0)
	man.doSleep()

	if !man.fired[0] {
		t.Error("slot 0 should have fired via external pin")
	}
}

func TestStep_PinSelectiveFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{7: true}, // pin 8 did not fire
	}
	groups := []Group{
		{Name: "a", Interval: 0},
		{Name: "b", Interval: 0},
		{Name: "c", Interval: 0},
	}
	man, _ := newTestManager(sys, groups, nil)
	man.RegisterExternalPin(7, 0)
	man.RegisterExternalPin(8, 2)
	man.doSleep()

	if !man.fired[0] || man.fired[1] || man.fired[2] {
		t.Errorf("fired = %v, want [true false false]", man.fired)
	}
}

func TestStep_DeadlineAndPinSimultaneous(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(11 * time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	groups := []Group{
		{Name: "timed", Interval: 10 * time.Second},
		{Name: "ext", Interval: 0},
	}
	man, _ := newTestManager(sys, groups, nil)
	man.RegisterExternalPin(7, 1)
	man.doSleep()

	if !man.fired[0] {
		t.Error("slot 0 (timer) should have fired")
	}
	if !man.fired[1] {
		t.Error("slot 1 (external) should have fired")
	}
}

func TestDoSleep_TargetPassedToSleep(t *testing.T) {
	var capturedTarget time.Time
	sys := &mockSystem{
		sleepFn: func(target time.Time) (time.Time, error) {
			capturedTarget = target
			return T.Add(6 * time.Second), nil
		},
	}
	groups := []Group{{Name: "a", Interval: 5 * time.Second}}
	man, _ := newTestManager(sys, groups, nil)
	man.doSleep()

	want := T.Add(5 * time.Second)
	if !capturedTarget.Equal(want) {
		t.Errorf("Sleep called with target %v, want %v", capturedTarget, want)
	}
}

// --- step() tests ---

func TestStep_GroupWithSensorsFires(t *testing.T) {
	dev := &mockSensor{
		id:       "test-sensor",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	recorder := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{
		Name:     "weather",
		Interval: 5 * time.Second,
		Sensors:  []sensor.Sensor{dev},
	}}

	man, _ := newTestManager(sys, groups, []sensor.Recorder{recorder})
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor.Measure() called %d times, want 1", dev.measureCalls)
	}
	if len(recorder.readings) != 1 {
		t.Fatalf("recorder got %d readings, want 1", len(recorder.readings))
	}
	if recorder.readings[0].Label != "temp" {
		t.Errorf("label = %q, want temp", recorder.readings[0].Label)
	}
	if recorder.measIDs[0] != "test-sensor" {
		t.Errorf("id = %q, want test-sensor", recorder.measIDs[0])
	}
}

func TestStep_SharedSensorMeasuredOnce(t *testing.T) {
	dev := &mockSensor{
		id:       "shared-sensor",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	recorder := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{
		{Name: "fast", Interval: 5 * time.Second, Sensors: []sensor.Sensor{dev}},
		{Name: "medium", Interval: 5 * time.Second, Sensors: []sensor.Sensor{dev}},
	}

	man, mo := newTestManager(sys, groups, []sensor.Recorder{recorder})
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor.Measure() called %d times, want 1 (shared sensor)", dev.measureCalls)
	}
	if len(recorder.readings) != 1 {
		t.Errorf("recorder got %d readings, want 1", len(recorder.readings))
	}
	if !mo.hasLog("wake: fast medium") {
		t.Errorf("expected combined groups log, got: %v", mo.logEntries)
	}
}

func TestStep_DifferentSensorsBothMeasured(t *testing.T) {
	temp := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	humidity := &mockSensor{
		id:       "humidity",
		readings: []sensor.Reading{{Label: "rh", Value: 5500, Unit: "mRH"}},
	}
	recorder := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{
		{Name: "fast", Interval: 5 * time.Second, Sensors: []sensor.Sensor{temp}},
		{Name: "medium", Interval: 5 * time.Second, Sensors: []sensor.Sensor{humidity}},
	}

	man, _ := newTestManager(sys, groups, []sensor.Recorder{recorder})
	man.step()

	if temp.measureCalls != 1 {
		t.Errorf("temp measured %d times, want 1", temp.measureCalls)
	}
	if humidity.measureCalls != 1 {
		t.Errorf("humidity measured %d times, want 1", humidity.measureCalls)
	}
	if len(recorder.readings) != 2 {
		t.Errorf("recorder got %d readings, want 2", len(recorder.readings))
	}
}

func TestStep_SharedSensorOnlyFiredGroupsCounted(t *testing.T) {
	dev := &mockSensor{
		id:       "sensor",
		readings: []sensor.Reading{{Label: "x", Value: 1, Unit: "u"}},
	}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	// Only "medium" has an interval — only medium fires.
	groups := []Group{
		{Name: "fast", Interval: 0, Sensors: []sensor.Sensor{dev}},
		{Name: "medium", Interval: 5 * time.Second, Sensors: []sensor.Sensor{dev}},
	}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor measured %d times, want 1", dev.measureCalls)
	}
	if !mo.hasLog("wake: medium") {
		t.Errorf("expected groups log with medium, got: %v", mo.logEntries)
	}
}

func TestStep_GroupWithHostFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{
		Name:     "heartbeat",
		Interval: 5 * time.Second,
		Host:     "http://localhost:4000",
		Payload:  config.PayloadFull,
	}}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("wake: heartbeat") {
		t.Errorf("expected groups log, got: %v", mo.logEntries)
	}
	if !mo.hasLog("payload: http://localhost:4000") {
		t.Errorf("expected payload log, got: %v", mo.logEntries)
	}
}

func TestStep_MultipleGroups(t *testing.T) {
	weatherSensor := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	rec := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{
		{
			Name:     "weather",
			Interval: 5 * time.Second,
			Sensors:  []sensor.Sensor{weatherSensor},
		},
		{
			Name:     "heartbeat",
			Interval: 5 * time.Second,
			Host:     "http://localhost",
			Payload:  config.PayloadMin,
		},
	}

	man, mo := newTestManager(sys, groups, []sensor.Recorder{rec})
	man.step()

	if weatherSensor.measureCalls != 1 {
		t.Errorf("weather sensor measured %d times, want 1", weatherSensor.measureCalls)
	}
	if !mo.hasLog("wake: weather heartbeat") {
		t.Errorf("expected groups log, got: %v", mo.logEntries)
	}
}

func TestStep_OnlyFiredGroupsRun(t *testing.T) {
	weatherSensor := &mockSensor{id: "temp"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{
		{
			Name:    "weather",
			// No Interval — weather does not fire via deadline.
			Sensors: []sensor.Sensor{weatherSensor},
		},
		{
			Name:     "heartbeat",
			Interval: 5 * time.Second,
			Host:     "http://localhost",
			Payload:  config.PayloadFull,
		},
	}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if weatherSensor.measureCalls != 0 {
		t.Error("weather sensor should not have been called (not fired)")
	}
	if !mo.hasLog("wake: heartbeat") {
		t.Error("heartbeat group should have run")
	}
}

func TestStep_ExternalWake(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	groups := []Group{{Name: "weather"}}
	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("external wake") {
		t.Errorf("expected external wake log, got: %v", mo.logEntries)
	}
}

func TestStep_SensorMeasureError(t *testing.T) {
	dev := &mockSensor{
		id:         "flaky-sensor",
		measureErr: errors.New("read timeout"),
	}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{
		Name:     "weather",
		Interval: 5 * time.Second,
		Sensors:  []sensor.Sensor{dev},
	}}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor.Measure() called %d times, want 1", dev.measureCalls)
	}
	if !mo.hasLog("failed to measure") {
		t.Errorf("expected measure error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_LEDOnPulseLEDGroup(t *testing.T) {
	led := &mockLED{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{Name: "sample", Interval: 5 * time.Second, PulseLED: true, Host: "http://x"}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(led)
	man.step()

	if !led.onCalled {
		t.Error("LED.On was not called")
	}
	if !led.offCalled {
		t.Error("LED.Off was not called")
	}
}

func TestStep_NoLEDWhenPulseLEDFalse(t *testing.T) {
	led := &mockLED{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{Name: "hb", Interval: 5 * time.Second, PulseLED: false, Host: "http://x"}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(led)
	man.step()

	if led.onCalled {
		t.Error("LED should not pulse when PulseLED is false")
	}
}

func TestStep_NoLEDOnExternalWake(t *testing.T) {
	led := &mockLED{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	// Group with PulseLED but no Interval — never fires via deadline.
	groups := []Group{{Name: "weather", PulseLED: true}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(led)
	man.step()

	if led.onCalled {
		t.Error("LED should not pulse on external wake (no groups fired)")
	}
}

func TestStep_PowerOnSensorRailsCalledWhenNeeded(t *testing.T) {
	dev := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "t", Value: 1, Unit: "C"}},
	}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{
		Name:     "weather",
		Interval: 5 * time.Second,
		Sensors:  []sensor.Sensor{dev},
	}}

	man, _ := newTestManager(sys, groups, nil)
	man.step()

	if sys.powerOnSensorRailsCalls != 1 {
		t.Errorf("PowerOnSensorRails called %d times, want 1", sys.powerOnSensorRailsCalls)
	}
}

func TestStep_NoSensorRailsForHostOnly(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{Name: "hb", Interval: 5 * time.Second, Host: "http://x", Payload: config.PayloadFull}}

	man, _ := newTestManager(sys, groups, nil)
	man.step()

	if sys.powerOnSensorRailsCalls != 0 {
		t.Errorf("PowerOnSensorRails called %d times, want 0 (no sensors)", sys.powerOnSensorRailsCalls)
	}
}

func TestStep_SleepError(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func(_ time.Time) (time.Time, error) {
			return time.Time{}, errors.New("standby failed")
		},
	}

	groups := []Group{{Name: "x", Host: "http://x"}}
	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("sleep: standby failed") {
		t.Errorf("expected sleep error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_FlushBeforeSleep(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	rec := &mockOutput{name: "rec"}
	groups := []Group{{Name: "x", Interval: 5 * time.Second, Host: "http://x"}}

	man, mo := newTestManager(sys, groups, []sensor.Recorder{rec})
	man.step()

	if !mo.flushCalled {
		t.Error("logger sink Flush() was not called before sleep")
	}
	if !rec.flushCalled {
		t.Error("recorder Flush() was not called before sleep")
	}
}

func TestStep_MultipleMeasurements(t *testing.T) {
	dev := &mockSensor{
		id: "multi",
		readings: []sensor.Reading{
			{Label: "temp", Value: 22000, Unit: "mC"},
			{Label: "rh", Value: 5500, Unit: "mRH"},
		},
	}
	rec := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{{
		Name:     "weather",
		Interval: 5 * time.Second,
		Sensors:  []sensor.Sensor{dev},
	}}

	man, _ := newTestManager(sys, groups, []sensor.Recorder{rec})
	man.step()

	if len(rec.readings) != 2 {
		t.Fatalf("recorder got %d readings, want 2", len(rec.readings))
	}
	if rec.readings[1].Label != "rh" {
		t.Errorf("readings[1].Label = %q, want rh", rec.readings[1].Label)
	}
}

func TestStep_NilCallbacks(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	groups := []Group{{Name: "x", Host: "http://x"}}
	man, _ := newTestManager(sys, groups, nil)
	man.step() // should not panic
}

func TestNew_DeduplicatesSensors(t *testing.T) {
	dev := &mockSensor{id: "shared"}

	groups := []Group{
		{Name: "a", Sensors: []sensor.Sensor{dev}},
		{Name: "b", Sensors: []sensor.Sensor{dev}},
	}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	man, _ := newTestManager(sys, groups, nil)

	if len(man.allSensors) != 1 {
		t.Errorf("allSensors = %d, want 1 (deduplicated)", len(man.allSensors))
	}
	if len(man.measured) != 1 {
		t.Errorf("measured = %d, want 1", len(man.measured))
	}
}
