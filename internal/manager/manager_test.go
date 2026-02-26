package manager

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fallback"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink"
)

var T = time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
var R = fallback.Rails{}

// --- mocks ---

type mockOutput struct {
	readings    []sensor.Reading
	measIDs     []string
	logEntries  []string
	flushCalled bool
}

func (m *mockOutput) Data(_ time.Time, id string, readings []sensor.Reading) error {
	m.measIDs = append(m.measIDs, id)
	m.readings = append(m.readings, readings...)
	return nil
}

func (m *mockOutput) Log(_ time.Time, _ config.LogLevel, msg string) error {
	m.logEntries = append(m.logEntries, msg)
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
	sleepFn   func(target time.Time) (time.Time, error)
	firedPins map[hal.Pin]bool
}

func (m *mockSystem) Sleep(target time.Time) (time.Time, error) {
	return m.sleepFn(target)
}

func (m *mockSystem) PinFired(pin hal.Pin) bool {
	return m.firedPins[pin]
}

type mockRails struct {
	powerCalls []hal.RailState
}

func (m *mockRails) Identifier() string { return "mockRails" }

func (m *mockRails) Power(state hal.RailState) {
	m.powerCalls = append(m.powerCalls, state)
}

type mockLED struct {
	blinkCalled bool
}

func (m *mockLED) On()          {}
func (m *mockLED) Off()         {}
func (m *mockLED) Blink()       { m.blinkCalled = true }
func (m *mockLED) Pin() hal.Pin { return hal.NoPin }

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

func newTestManager(sys *mockSystem, rails hal.Rails, groups []Group, sensors map[string]sensor.Sensor, dataSinks []sink.DataSink) (*Manager, *mockOutput) {
	mo := &mockOutput{}

	logger := log.NewLogger(time.Time{})
	logger.AddSink(mo, config.LogLevelDebug)

	// Construct Manager directly to preserve the caller-specified group ordering
	// (bit assignments in tests depend on slice position, not alphabetical sort).
	man := &Manager{
		sleeper:   sys,
		rails:     rails,
		led:       fallback.LED{},
		groups:    groups,
		sensors:   sensors,
		dataSinks: dataSinks,
		logger:    logger,
	}
	man.wakeTime = T
	for i := range man.groups {
		man.groups[i].deadline.init(T)
	}
	return man, mo
}

// makeGroup creates a simple periodic group (no sensors, no LED pulse).
func makeGroup(name string, interval time.Duration) Group {
	return BuildGroup(name, config.Group{Interval: interval})
}

// sensorGroup creates a group with sensors and an optional interval.
func sensorGroup(name string, interval time.Duration, sensorIDs ...string) Group {
	return BuildGroup(name, config.Group{Interval: interval, Sensors: sensorIDs})
}

// ledGroup creates a group with PulseLED=true.
func ledGroup(name string, interval time.Duration) Group {
	return BuildGroup(name, config.Group{Interval: interval, PulseLED: true})
}

// pinGroup creates a group triggered by an interrupt pin (no interval).
func pinGroup(name string, pins ...hal.Pin) Group {
	return BuildGroup(name, config.Group{InterruptPins: pins})
}

// sensorMap builds a sensor registry from the provided sensors.
func sensorMap(sensors ...*mockSensor) map[string]sensor.Sensor {
	m := make(map[string]sensor.Sensor, len(sensors))
	for _, s := range sensors {
		m[s.id] = s
	}
	return m
}

// --- deadline tracking tests ---

func TestEarliestDeadline(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{
		makeGroup("sample", 10*time.Second),
		makeGroup("heartbeat", 5*time.Second),
	}
	man, _ := newTestManager(sys, R, groups, nil, nil)

	// Set deadlines manually for precision.
	man.groups[0].deadline.next = T.Add(10 * time.Second)
	man.groups[1].deadline.next = T.Add(5 * time.Second)

	got := man.earliestDeadline()
	want := T.Add(5 * time.Second)
	if !got.Equal(want) {
		t.Errorf("earliestDeadline() = %v, want %v", got, want)
	}
}

func TestStep_GroupFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{makeGroup("sample", 10*time.Second)}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 == 0 {
		t.Error("group 0 should have fired")
	}
}

func TestStep_OnlyEarliestFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}
	// sample has 1m interval, heartbeat has 5s interval; only heartbeat fires at T+6s.
	groups := []Group{
		makeGroup("sample", time.Minute),
		makeGroup("heartbeat", 5*time.Second),
	}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 != 0 {
		t.Error("sample (1m) should not have fired")
	}
	if fired&2 == 0 {
		t.Error("heartbeat (5s) should have fired")
	}
}

func TestStep_BothFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{
		makeGroup("sample", 10*time.Second),
		makeGroup("heartbeat", 10*time.Second),
	}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 == 0 || fired&2 == 0 {
		t.Errorf("fired=%b, want both bits set", fired)
	}

	wantNext := T.Add(20 * time.Second)
	if !man.groups[0].deadline.next.Equal(wantNext) {
		t.Errorf("groups[0] deadline = %v, want %v", man.groups[0].deadline.next, wantNext)
	}
	if !man.groups[1].deadline.next.Equal(wantNext) {
		t.Errorf("groups[1] deadline = %v, want %v", man.groups[1].deadline.next, wantNext)
	}
}

func TestStep_DeadlineAdvances(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	groups := []Group{makeGroup("sample", 10*time.Second)}
	man, _ := newTestManager(sys, R, groups, nil, nil)

	fired := man.doSleep()
	if fired&1 == 0 {
		t.Fatal("first doSleep: group should have fired")
	}
	want := T.Add(20 * time.Second)
	if !man.groups[0].deadline.next.Equal(want) {
		t.Errorf("deadline after fire = %v, want %v", man.groups[0].deadline.next, want)
	}
}

func TestStep_ExternalPinFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	// Group triggered by interrupt pin only (no interval).
	groups := []Group{pinGroup("rain", hal.Pin(7))}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 == 0 {
		t.Error("rain group should have fired via external pin")
	}
}

func TestStep_ExtPinNoFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{},
	}
	groups := []Group{pinGroup("rain", hal.Pin(7))}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 != 0 {
		t.Error("rain group should not have fired (pin not active)")
	}
}

func TestStep_IntervalAndPinBothFireSameGroup(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(11 * time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	groups := []Group{
		BuildGroup("sample", config.Group{
			Interval:      10 * time.Second,
			InterruptPins: []hal.Pin{7},
		}),
	}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 == 0 {
		t.Error("sample group should have fired (both deadline and pin)")
	}
}

func TestStep_SharedInterruptPin(t *testing.T) {
	// Two groups share pin 7 — both should fire when pin fires.
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	groups := []Group{
		pinGroup("groupA", hal.Pin(7)),
		pinGroup("groupB", hal.Pin(7)),
	}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	fired := man.doSleep()

	if fired&1 == 0 {
		t.Error("groupA should have fired (shares pin 7)")
	}
	if fired&2 == 0 {
		t.Error("groupB should have fired (shares pin 7)")
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
	groups := []Group{makeGroup("sample", 5*time.Second)}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	man.doSleep()

	want := T.Add(5 * time.Second)
	if !capturedTarget.Equal(want) {
		t.Errorf("Sleep called with target %v, want %v", capturedTarget, want)
	}
}

// --- step() tests ---

func TestStep_SensorMeasured(t *testing.T) {
	dev := &mockSensor{
		id:       "test-sensor",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	recorder := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{sensorGroup("sample", 5*time.Second, "test-sensor")}
	man, _ := newTestManager(sys, R, groups, sensorMap(dev), []sink.DataSink{recorder})
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

func TestStep_MultipleSensorsBothMeasured(t *testing.T) {
	temp := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	humidity := &mockSensor{
		id:       "humidity",
		readings: []sensor.Reading{{Label: "rh", Value: 5500, Unit: "mRH"}},
	}
	recorder := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{sensorGroup("sample", 5*time.Second, "temp", "humidity")}
	man, _ := newTestManager(sys, R, groups, sensorMap(temp, humidity), []sink.DataSink{recorder})
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

func TestStep_TwoGroupsBothFire_WakeLog(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	weatherSensor := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	rec := &mockOutput{}

	groups := []Group{
		sensorGroup("sample", 5*time.Second, "temp"),
		ledGroup("heartbeat", 5*time.Second),
	}
	man, mo := newTestManager(sys, R, groups, sensorMap(weatherSensor), []sink.DataSink{rec})
	man.step()

	if weatherSensor.measureCalls != 1 {
		t.Errorf("weather sensor measured %d times, want 1", weatherSensor.measureCalls)
	}
	if !mo.hasLog("wake: sample heartbeat") {
		t.Errorf("expected combined wake log, got: %v", mo.logEntries)
	}
}

func TestStep_OnlyLEDGroupFires_SensorsNotMeasured(t *testing.T) {
	weatherSensor := &mockSensor{id: "temp"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	// "heartbeat" group fires (5s), "sample" group has 1h interval (doesn't fire).
	groups := []Group{
		sensorGroup("sample", time.Hour, "temp"),
		ledGroup("heartbeat", 5*time.Second),
	}
	man, mo := newTestManager(sys, R, groups, sensorMap(weatherSensor), nil)
	man.step()

	if weatherSensor.measureCalls != 0 {
		t.Error("temp sensor should not have been measured (sample group didn't fire)")
	}
	if !mo.hasLog("wake: heartbeat") {
		t.Error("heartbeat should appear in wake log")
	}
}

func TestStep_ExternalWake(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	// Group has long interval, doesn't fire at T.
	groups := []Group{makeGroup("sample", time.Hour)}
	man, mo := newTestManager(sys, R, groups, nil, nil)
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

	groups := []Group{sensorGroup("sample", 5*time.Second, "flaky-sensor")}
	man, mo := newTestManager(sys, R, groups, sensorMap(dev), nil)
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor.Measure() called %d times, want 1", dev.measureCalls)
	}
	if !mo.hasLog("measure: flaky-sensor: read timeout") {
		t.Errorf("expected measure error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_LEDBlinksWhenPulseLEDGroupFires(t *testing.T) {
	mockled := &mockLED{}
	sys := &mockSystem{sleepFn: afterDeadlineSleep(T.Add(6 * time.Second))}
	logger := log.NewLogger(time.Time{})

	groups := map[string]config.Group{
		"heartbeat": {Interval: 5 * time.Second, PulseLED: true},
	}
	man := New(sys, R, mockled, groups, nil, nil, logger)
	man.wakeTime = T
	for i := range man.groups {
		man.groups[i].deadline.init(T)
	}
	man.step()

	if !mockled.blinkCalled {
		t.Error("LED.Blink() was not called when pulse_led group fired")
	}
}

func TestStep_LEDNoBlinkWhenNoPulseLEDGroup(t *testing.T) {
	mockled := &mockLED{}
	sys := &mockSystem{sleepFn: afterDeadlineSleep(T.Add(6 * time.Second))}
	logger := log.NewLogger(time.Time{})

	// Group fires but has no PulseLED.
	groups := map[string]config.Group{
		"sample": {Interval: 5 * time.Second},
	}
	man := New(sys, R, mockled, groups, nil, nil, logger)
	man.wakeTime = T
	for i := range man.groups {
		man.groups[i].deadline.init(T)
	}
	man.step()

	if mockled.blinkCalled {
		t.Error("LED.Blink() should not be called when no pulse_led group fired")
	}
}

func TestStep_SensorRailsCycleAroundMeasurement(t *testing.T) {
	dev := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "t", Value: 1, Unit: "C"}},
	}
	rails := &mockRails{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{sensorGroup("sample", 5*time.Second, "temp")}
	man, _ := newTestManager(sys, rails, groups, sensorMap(dev), nil)
	man.step()

	// Expect RailsFull (on) then RailsCore (off) around the measurement.
	want := []hal.RailState{hal.RailsFull, hal.RailsCore}
	if len(rails.powerCalls) != 2 || rails.powerCalls[0] != want[0] || rails.powerCalls[1] != want[1] {
		t.Errorf("Power calls = %v, want %v", rails.powerCalls, want)
	}
}

func TestStep_NoRailsWhenNoFiredGroupHasSensors(t *testing.T) {
	rails := &mockRails{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	// LED group fires but has no sensors.
	groups := []Group{ledGroup("heartbeat", 5*time.Second)}
	man, _ := newTestManager(sys, rails, groups, nil, nil)
	man.step()

	if len(rails.powerCalls) != 0 {
		t.Errorf("Power called %d times, want 0 (no sensors in fired group)", len(rails.powerCalls))
	}
}

func TestStep_ORLogic_RailsFiredOnce(t *testing.T) {
	// Group A fires and has sensors; group B fires but has no sensors.
	// Rails should cycle exactly once.
	dev := &mockSensor{
		id:       "vbat",
		readings: []sensor.Reading{{Label: "voltage", Value: 3300, Unit: "mV"}},
	}
	rails := &mockRails{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}

	groups := []Group{
		sensorGroup("sample", 10*time.Second, "vbat"), // has sensors
		ledGroup("heartbeat", 10*time.Second),          // no sensors, has PulseLED
	}
	man, _ := newTestManager(sys, rails, groups, sensorMap(dev), nil)
	man.step()

	// Despite two groups firing, rails should cycle exactly once.
	want := []hal.RailState{hal.RailsFull, hal.RailsCore}
	if len(rails.powerCalls) != 2 || rails.powerCalls[0] != want[0] || rails.powerCalls[1] != want[1] {
		t.Errorf("Power calls = %v, want exactly [Full Core]", rails.powerCalls)
	}
}

func TestStep_ORLogic_LEDFiredBySecondGroup(t *testing.T) {
	// Group A fires (no LED), group B fires (PulseLED=true) → LED blinks.
	mockled := &mockLED{}
	sys := &mockSystem{sleepFn: afterDeadlineSleep(T.Add(11 * time.Second))}
	logger := log.NewLogger(time.Time{})

	groups := map[string]config.Group{
		"sample":    {Interval: 10 * time.Second},               // no PulseLED
		"heartbeat": {Interval: 10 * time.Second, PulseLED: true}, // PulseLED=true
	}
	man := New(sys, R, mockled, groups, nil, nil, logger)
	man.wakeTime = T
	for i := range man.groups {
		man.groups[i].deadline.init(T)
	}
	man.step()

	if !mockled.blinkCalled {
		t.Error("LED.Blink() should be called when any fired group has PulseLED=true")
	}
}

func TestStep_SensorDedup_TwoGroupsSameSensor(t *testing.T) {
	// Both groups list "vbat". Both fire. vbat should be measured exactly once.
	dev := &mockSensor{
		id:       "vbat",
		readings: []sensor.Reading{{Label: "voltage", Value: 3300, Unit: "mV"}},
	}
	rec := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}

	groups := []Group{
		sensorGroup("groupA", 10*time.Second, "vbat"),
		sensorGroup("groupB", 10*time.Second, "vbat"),
	}
	man, _ := newTestManager(sys, R, groups, sensorMap(dev), []sink.DataSink{rec})
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("vbat.Measure() called %d times, want 1 (dedup)", dev.measureCalls)
	}
	if len(rec.readings) != 1 {
		t.Errorf("recorder got %d readings, want 1", len(rec.readings))
	}
}

func TestStep_SensorDedup_PartialOverlap(t *testing.T) {
	// GroupA fires: sensors [temp, vbat]. GroupB fires: sensors [vbat, humidity].
	// Union = [temp, vbat, humidity]; each measured exactly once.
	temp := &mockSensor{id: "temp", readings: []sensor.Reading{{Label: "t", Value: 1, Unit: "C"}}}
	vbat := &mockSensor{id: "vbat", readings: []sensor.Reading{{Label: "v", Value: 2, Unit: "mV"}}}
	hum := &mockSensor{id: "humidity", readings: []sensor.Reading{{Label: "rh", Value: 3, Unit: "%"}}}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}

	groups := []Group{
		sensorGroup("groupA", 10*time.Second, "temp", "vbat"),
		sensorGroup("groupB", 10*time.Second, "vbat", "humidity"),
	}
	man, _ := newTestManager(sys, R, groups, sensorMap(temp, vbat, hum), nil)
	man.step()

	if temp.measureCalls != 1 {
		t.Errorf("temp measured %d times, want 1", temp.measureCalls)
	}
	if vbat.measureCalls != 1 {
		t.Errorf("vbat measured %d times, want 1 (dedup)", vbat.measureCalls)
	}
	if hum.measureCalls != 1 {
		t.Errorf("humidity measured %d times, want 1", hum.measureCalls)
	}
}

func TestStep_SleepError(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func(_ time.Time) (time.Time, error) {
			return time.Time{}, errors.New("standby failed")
		},
	}

	groups := []Group{makeGroup("sample", 5*time.Second)}
	man, mo := newTestManager(sys, R, groups, nil, nil)
	man.step()

	if !mo.hasLog("sleep: standby failed") {
		t.Errorf("expected sleep error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_FlushBeforeSleep(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	rec := &mockOutput{}
	groups := []Group{makeGroup("sample", 5*time.Second)}
	man, mo := newTestManager(sys, R, groups, nil, []sink.DataSink{rec})
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
	rec := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{sensorGroup("sample", 5*time.Second, "multi")}
	man, _ := newTestManager(sys, R, groups, sensorMap(dev), []sink.DataSink{rec})
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

	groups := []Group{makeGroup("sample", time.Hour)}
	man, _ := newTestManager(sys, R, groups, nil, nil)
	man.step() // should not panic
}

func TestStep_SensorNotInRegistry_Skipped(t *testing.T) {
	// Group lists a sensor ID that's not in the registry — no panic, no measurement.
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	groups := []Group{sensorGroup("sample", 5*time.Second, "unknown-sensor")}
	man, _ := newTestManager(sys, R, groups, map[string]sensor.Sensor{}, nil)
	man.step() // should not panic or error
}
