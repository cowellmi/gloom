package manager

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// --- mocks ---

type mockOutput struct {
	name         string
	measurements []sensor.Measurement
	measDevice   string
	logEntries   []string
	flushCalled  bool
}

func (m *mockOutput) Name() string { return m.name }

func (m *mockOutput) Record(_ time.Time, device string, ms []sensor.Measurement) error {
	m.measDevice = device
	m.measurements = append(m.measurements, ms...)
	return nil
}

func (m *mockOutput) WriteLog(_ time.Time, _ log.Level, msg string) error {
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

type mockSystem struct {
	sleepFn          func() ([]bool, error)
	timeFn           func() (time.Time, error)
	nextWakeDuration time.Duration
	sensorRailsCalls int
}

func (m *mockSystem) ReadTime() (time.Time, error) {
	return m.timeFn()
}

func (m *mockSystem) Sleep() ([]bool, error) {
	return m.sleepFn()
}

func (m *mockSystem) NextWake() time.Duration {
	return m.nextWakeDuration
}

func (m *mockSystem) EnableSensorRails() {
	m.sensorRailsCalls++
}

type mockSensor struct {
	name          string
	initErr       error
	measurements  []sensor.Measurement
	measureErr    error
	initCalled    bool
	measureCalled bool
}

func (m *mockSensor) Name() string { return m.name }

func (m *mockSensor) Init() error {
	m.initCalled = true
	return m.initErr
}

func (m *mockSensor) Measure() ([]sensor.Measurement, error) {
	m.measureCalled = true
	return m.measurements, m.measureErr
}

// --- helpers ---

func fixedTime() (time.Time, error) {
	return time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC), nil
}

func newTestManager(sys *mockSystem, groups []Group, recorders []sensor.Recorder) (*Manager, *mockOutput) {
	mo := &mockOutput{name: "test"}

	logger := log.NewLogger(time.Time{})
	logger.AddSink(mo, log.LevelDebug)

	man := New(sys, groups, recorders, logger)
	return man, mo
}

// --- tests ---

func TestStep_GroupWithSensorsFires(t *testing.T) {
	dev := &mockSensor{
		name: "test-sensor",
		measurements: []sensor.Measurement{
			{Label: "temp", Value: "22", Unit: "C"},
		},
	}
	recorder := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "weather",
		Sensors: []sensor.Device{dev},
	}}

	man, _ := newTestManager(sys, groups, []sensor.Recorder{recorder})
	man.step()

	if !dev.initCalled {
		t.Error("sensor.Init() was not called")
	}
	if !dev.measureCalled {
		t.Error("sensor.Measure() was not called")
	}
	if len(recorder.measurements) != 1 {
		t.Fatalf("recorder got %d measurements, want 1", len(recorder.measurements))
	}
	if recorder.measurements[0].Label != "temp" {
		t.Errorf("label = %q, want temp", recorder.measurements[0].Label)
	}
	if recorder.measDevice != "test-sensor" {
		t.Errorf("device = %q, want test-sensor", recorder.measDevice)
	}
}

func TestStep_GroupWithHostFires(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "heartbeat",
		Host:    "http://localhost:4000",
		Payload: config.PayloadFull,
	}}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("group: heartbeat") {
		t.Errorf("expected group log, got: %v", mo.logEntries)
	}
	if !mo.hasLog("payload: http://localhost:4000") {
		t.Errorf("expected payload log, got: %v", mo.logEntries)
	}
}

func TestStep_MultipleGroups(t *testing.T) {
	weatherSensor := &mockSensor{
		name:         "temp",
		measurements: []sensor.Measurement{{Label: "temp", Value: "22", Unit: "C"}},
	}
	rec := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true, true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{
		{
			Name:    "weather",
			Sensors: []sensor.Device{weatherSensor},
		},
		{
			Name:    "heartbeat",
			Host:    "http://localhost",
			Payload: config.PayloadMin,
		},
	}

	man, mo := newTestManager(sys, groups, []sensor.Recorder{rec})
	man.step()

	if !weatherSensor.measureCalled {
		t.Error("weather sensor should have been measured")
	}
	if !mo.hasLog("group: heartbeat") {
		t.Error("heartbeat group should have been logged")
	}
}

func TestStep_OnlyFiredGroupsRun(t *testing.T) {
	weatherSensor := &mockSensor{name: "temp"}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{false, true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{
		{
			Name:    "weather",
			Sensors: []sensor.Device{weatherSensor},
		},
		{
			Name:    "heartbeat",
			Host:    "http://localhost",
			Payload: config.PayloadFull,
		},
	}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if weatherSensor.initCalled {
		t.Error("weather sensor should not have been called (not fired)")
	}
	if !mo.hasLog("group: heartbeat") {
		t.Error("heartbeat group should have run")
	}
}

func TestStep_ExternalWake(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{false}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "weather"}}
	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("external wake") {
		t.Errorf("expected external wake log, got: %v", mo.logEntries)
	}
}

func TestStep_SensorInitError(t *testing.T) {
	dev := &mockSensor{
		name:    "bad-sensor",
		initErr: errors.New("init failed"),
	}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "weather",
		Sensors: []sensor.Device{dev},
	}}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !dev.initCalled {
		t.Error("sensor.Init() was not called")
	}
	if dev.measureCalled {
		t.Error("sensor.Measure() should not be called after init error")
	}
	if !mo.hasLog("failed to initialize") {
		t.Errorf("expected init error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_SensorMeasureError(t *testing.T) {
	dev := &mockSensor{
		name:       "flaky-sensor",
		measureErr: errors.New("read timeout"),
	}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "weather",
		Sensors: []sensor.Device{dev},
	}}

	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !dev.measureCalled {
		t.Error("sensor.Measure() was not called")
	}
	if !mo.hasLog("failed to measure") {
		t.Errorf("expected measure error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_LEDOnPulseLEDGroup(t *testing.T) {
	var ledOnCalled, ledOffCalled bool

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "sample", PulseLED: true, Host: "http://x"}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(func() { ledOnCalled = true }, func() { ledOffCalled = true })
	man.step()

	if !ledOnCalled {
		t.Error("LEDOn callback was not called")
	}
	if !ledOffCalled {
		t.Error("LEDOff callback was not called")
	}
}

func TestStep_NoLEDWhenPulseLEDFalse(t *testing.T) {
	var ledOnCalled bool

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "hb", PulseLED: false, Host: "http://x"}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(func() { ledOnCalled = true }, func() {})
	man.step()

	if ledOnCalled {
		t.Error("LED should not pulse when PulseLED is false")
	}
}

func TestStep_NoLEDOnExternalWake(t *testing.T) {
	var ledOnCalled bool

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{false}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "weather", PulseLED: true}}

	man, _ := newTestManager(sys, groups, nil)
	man.SetLED(func() { ledOnCalled = true }, func() {})
	man.step()

	if ledOnCalled {
		t.Error("LED should not pulse on external wake (no groups fired)")
	}
}

func TestStep_EnableSensorRailsCalledWhenNeeded(t *testing.T) {
	dev := &mockSensor{name: "temp", measurements: []sensor.Measurement{{Label: "t", Value: "1", Unit: "C"}}}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "weather",
		Sensors: []sensor.Device{dev},
	}}

	man, _ := newTestManager(sys, groups, nil)
	man.step()

	if sys.sensorRailsCalls != 1 {
		t.Errorf("EnableSensorRails called %d times, want 1", sys.sensorRailsCalls)
	}
}

func TestStep_NoSensorRailsForHostOnly(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "hb", Host: "http://x", Payload: config.PayloadFull}}

	man, _ := newTestManager(sys, groups, nil)
	man.step()

	if sys.sensorRailsCalls != 0 {
		t.Errorf("EnableSensorRails called %d times, want 0 (no sensors)", sys.sensorRailsCalls)
	}
}

func TestStep_ReadTimeError(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{false}, nil },
		timeFn: func() (time.Time, error) {
			return time.Time{}, errors.New("rtc dead")
		},
	}

	groups := []Group{{Name: "x", Host: "http://x"}}
	man, mo := newTestManager(sys, groups, nil)
	man.step()

	if !mo.hasLog("rtc:") {
		t.Errorf("expected rtc error in logs, got: %v", mo.logEntries)
	}
}

func TestStep_SleepError(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) {
			return []bool{false}, errors.New("standby failed")
		},
		timeFn: fixedTime,
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
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	rec := &mockOutput{name: "rec"}
	groups := []Group{{Name: "x", Host: "http://x"}}

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
		name: "multi",
		measurements: []sensor.Measurement{
			{Label: "temp", Value: "22", Unit: "C"},
			{Label: "rh", Value: "55", Unit: "%"},
		},
	}
	rec := &mockOutput{name: "rec"}

	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{true}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{
		Name:    "weather",
		Sensors: []sensor.Device{dev},
	}}

	man, _ := newTestManager(sys, groups, []sensor.Recorder{rec})
	man.step()

	if len(rec.measurements) != 2 {
		t.Fatalf("recorder got %d measurements, want 2", len(rec.measurements))
	}
	if rec.measurements[1].Label != "rh" {
		t.Errorf("measurement[1].Label = %q, want rh", rec.measurements[1].Label)
	}
}

func TestStep_NilCallbacks(t *testing.T) {
	sys := &mockSystem{
		sleepFn: func() ([]bool, error) { return []bool{false}, nil },
		timeFn:  fixedTime,
	}

	groups := []Group{{Name: "x", Host: "http://x"}}
	man, _ := newTestManager(sys, groups, nil)
	man.step() // should not panic
}
