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
	"github.com/cowellmi/gloom/internal/sink"
)

var T = time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

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

func newTestManager(sys *mockSystem, cfg config.Config, sensors []sensor.Sensor, dataSinks []sink.DataSink) (*Manager, *mockOutput) {
	mo := &mockOutput{}

	logger := log.NewLogger(time.Time{})
	logger.AddSink(mo, config.LogLevelDebug)

	man := New(sys, cfg, sensors, dataSinks, logger)
	man.wakeTime = T
	man.sampleDeadline.init(T)
	man.hbDeadline.init(T)
	return man, mo
}

// --- deadline tracking tests ---

func TestEarliestDeadline(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	cfg := config.Config{
		SampleInterval:    10 * time.Second,
		HeartbeatInterval: 5 * time.Second,
	}
	man, _ := newTestManager(sys, cfg, nil, nil)

	// Set deadlines manually for precision.
	man.sampleDeadline.next = T.Add(10 * time.Second)
	man.hbDeadline.next = T.Add(5 * time.Second)

	got := man.earliestDeadline()
	want := T.Add(5 * time.Second)
	if !got.Equal(want) {
		t.Errorf("earliestDeadline() = %v, want %v", got, want)
	}
}

func TestStep_SampleFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	cfg := config.Config{SampleInterval: 10 * time.Second}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, hbFired := man.doSleep()

	if !sampleFired {
		t.Error("sample should have fired")
	}
	if hbFired {
		t.Error("heartbeat should not have fired (disabled)")
	}
}

func TestStep_HeartbeatFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}
	cfg := config.Config{HeartbeatInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, hbFired := man.doSleep()

	if sampleFired {
		t.Error("sample should not have fired (disabled)")
	}
	if !hbFired {
		t.Error("heartbeat should have fired")
	}
}

func TestStep_OnlyEarliestFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}
	// sample has 1m interval, hb has 5s interval; only hb fires at T+6s.
	cfg := config.Config{
		SampleInterval:    time.Minute,
		HeartbeatInterval: 5 * time.Second,
	}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, hbFired := man.doSleep()

	if sampleFired {
		t.Error("sample (1m) should not have fired")
	}
	if !hbFired {
		t.Error("heartbeat (5s) should have fired")
	}
}

func TestStep_BothFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	cfg := config.Config{
		SampleInterval:    10 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, hbFired := man.doSleep()

	if !sampleFired || !hbFired {
		t.Errorf("sampleFired=%v hbFired=%v, want both true", sampleFired, hbFired)
	}

	wantNext := T.Add(20 * time.Second)
	if !man.sampleDeadline.next.Equal(wantNext) {
		t.Errorf("sampleDeadline = %v, want %v", man.sampleDeadline.next, wantNext)
	}
	if !man.hbDeadline.next.Equal(wantNext) {
		t.Errorf("hbDeadline = %v, want %v", man.hbDeadline.next, wantNext)
	}
}

func TestStep_DeadlineAdvances(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(11 * time.Second)),
	}
	cfg := config.Config{SampleInterval: 10 * time.Second}
	man, _ := newTestManager(sys, cfg, nil, nil)

	sampleFired, _ := man.doSleep()
	if !sampleFired {
		t.Fatal("first doSleep: sample should have fired")
	}
	want := T.Add(20 * time.Second)
	if !man.sampleDeadline.next.Equal(want) {
		t.Errorf("sampleDeadline after fire = %v, want %v", man.sampleDeadline.next, want)
	}
}

func TestStep_ExternalPinFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	// Sample with no interval — fires only via interrupt pin.
	cfg := config.Config{InterruptPins: []hal.Pin{7}}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, _ := man.doSleep()

	if !sampleFired {
		t.Error("sample should have fired via external pin")
	}
}

func TestStep_ExtPinNoFire(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(time.Second)),
		firedPins: map[hal.Pin]bool{},
	}
	cfg := config.Config{InterruptPins: []hal.Pin{7}}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, _ := man.doSleep()

	if sampleFired {
		t.Error("sample should not have fired (pin not active)")
	}
}

func TestStep_SampleAndHbSimultaneous(t *testing.T) {
	sys := &mockSystem{
		sleepFn:   afterDeadlineSleep(T.Add(11 * time.Second)),
		firedPins: map[hal.Pin]bool{7: true},
	}
	cfg := config.Config{
		SampleInterval:    10 * time.Second,
		InterruptPins:     []hal.Pin{7},
		HeartbeatInterval: 10 * time.Second,
	}
	man, _ := newTestManager(sys, cfg, nil, nil)
	sampleFired, hbFired := man.doSleep()

	if !sampleFired {
		t.Error("sample should have fired")
	}
	if !hbFired {
		t.Error("heartbeat should have fired")
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
	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, nil, nil)
	man.doSleep()

	want := T.Add(5 * time.Second)
	if !capturedTarget.Equal(want) {
		t.Errorf("Sleep called with target %v, want %v", capturedTarget, want)
	}
}

// --- step() tests ---

func TestStep_SampleSensorMeasured(t *testing.T) {
	dev := &mockSensor{
		id:       "test-sensor",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	recorder := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, []sensor.Sensor{dev}, []sink.DataSink{recorder})
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

	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, []sensor.Sensor{temp, humidity}, []sink.DataSink{recorder})
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

func TestStep_SampleAndHeartbeatFire(t *testing.T) {
	weatherSensor := &mockSensor{
		id:       "temp",
		readings: []sensor.Reading{{Label: "temp", Value: 22000, Unit: "mC"}},
	}
	rec := &mockOutput{}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	cfg := config.Config{
		SampleInterval:    5 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatPayload:  config.HeartbeatPayloadMin,
	}
	man, mo := newTestManager(sys, cfg, []sensor.Sensor{weatherSensor}, []sink.DataSink{rec})
	man.step()

	if weatherSensor.measureCalls != 1 {
		t.Errorf("weather sensor measured %d times, want 1", weatherSensor.measureCalls)
	}
	if !mo.hasLog("wake: sample heartbeat") {
		t.Errorf("expected combined wake log, got: %v", mo.logEntries)
	}
}

func TestStep_OnlyHeartbeatFires(t *testing.T) {
	weatherSensor := &mockSensor{id: "temp"}

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	// Sample has no interval and no ext pin — never fires.
	cfg := config.Config{
		HeartbeatInterval: 5 * time.Second,
		HeartbeatPayload:  config.HeartbeatPayloadFull,
	}
	man, mo := newTestManager(sys, cfg, []sensor.Sensor{weatherSensor}, nil)
	man.step()

	if weatherSensor.measureCalls != 0 {
		t.Error("weather sensor should not have been measured (sample not fired)")
	}
	if !mo.hasLog("wake: heartbeat") {
		t.Error("heartbeat should appear in wake log")
	}
}

func TestStep_ExternalWake(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	man, mo := newTestManager(sys, config.Config{}, nil, nil)
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

	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, mo := newTestManager(sys, cfg, []sensor.Sensor{dev}, nil)
	man.step()

	if dev.measureCalls != 1 {
		t.Errorf("sensor.Measure() called %d times, want 1", dev.measureCalls)
	}
	if !mo.hasLog("measure: flaky-sensor: read timeout") {
		t.Errorf("expected measure error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_LEDOnBlinkLED(t *testing.T) {
	var blinked bool

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	cfg := config.Config{HeartbeatInterval: 5 * time.Second, HeartbeatLedPin: hal.Pin(16)}
	man, _ := newTestManager(sys, cfg, nil, nil)
	man.SetBlinkLED(func() { blinked = true })
	man.step()

	if !blinked {
		t.Error("blinkLED was not called")
	}
}

func TestStep_NoLEDWhenLedPinNone(t *testing.T) {
	var blinked bool

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	cfg := config.Config{HeartbeatInterval: 5 * time.Second, HeartbeatLedPin: hal.NoPin}
	man, _ := newTestManager(sys, cfg, nil, nil)
	man.SetBlinkLED(func() { blinked = true })
	man.step()

	if blinked {
		t.Error("blinkLED should not be called when LedPin is NoPin")
	}
}

func TestStep_NoLEDWhenHeartbeatDisabled(t *testing.T) {
	var blinked bool

	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T),
	}

	// Heartbeat disabled (Interval=0) — never fires — no blink regardless of LedPin.
	cfg := config.Config{HeartbeatLedPin: hal.Pin(16)}
	man, _ := newTestManager(sys, cfg, nil, nil)
	man.SetBlinkLED(func() { blinked = true })
	man.step()

	if blinked {
		t.Error("blinkLED should not be called when heartbeat is disabled")
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

	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, []sensor.Sensor{dev}, nil)
	man.step()

	if sys.powerOnSensorRailsCalls != 1 {
		t.Errorf("PowerOnSensorRails called %d times, want 1", sys.powerOnSensorRailsCalls)
	}
}

func TestStep_NoSensorRailsWhenNoSensors(t *testing.T) {
	sys := &mockSystem{
		sleepFn: afterDeadlineSleep(T.Add(6 * time.Second)),
	}

	cfg := config.Config{HeartbeatInterval: 5 * time.Second, HeartbeatPayload: config.HeartbeatPayloadFull}
	man, _ := newTestManager(sys, cfg, nil, nil)
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

	man, mo := newTestManager(sys, config.Config{}, nil, nil)
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
	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, mo := newTestManager(sys, cfg, nil, []sink.DataSink{rec})
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

	cfg := config.Config{SampleInterval: 5 * time.Second}
	man, _ := newTestManager(sys, cfg, []sensor.Sensor{dev}, []sink.DataSink{rec})
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

	man, _ := newTestManager(sys, config.Config{}, nil, nil)
	man.step() // should not panic
}
