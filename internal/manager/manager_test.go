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

// --- mocks ---

type mockSink struct {
	name         string
	measurements []sensor.Measurement
	measDevice   string
	logEntries   []string
	flushCalled  bool
}

func (m *mockSink) Name() string { return m.name }

func (m *mockSink) WriteMeasurements(_ []byte, _ time.Time, device string, ms []sensor.Measurement) error {
	m.measDevice = device
	m.measurements = append(m.measurements, ms...)
	return nil
}

func (m *mockSink) WriteLog(_ []byte, _ time.Time, _ log.Level, msg string) error {
	m.logEntries = append(m.logEntries, msg)
	return nil
}

func (m *mockSink) Flush() error {
	m.flushCalled = true
	return nil
}

func (m *mockSink) hasLog(substr string) bool {
	for _, e := range m.logEntries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

type mockSystem struct {
	name    string
	sleepFn func(sample, heartbeat time.Duration) (hal.WakeReason, error)
	timeFn  func() (time.Time, error)
}

func (m *mockSystem) Identifier() string { return m.name }

func (m *mockSystem) ReadTime() (time.Time, error) {
	return m.timeFn()
}

func (m *mockSystem) Sleep(s, h time.Duration) (hal.WakeReason, error) {
	return m.sleepFn(s, h)
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

func sampleWake(_, _ time.Duration) (hal.WakeReason, error) {
	return hal.WakeSample, nil
}

func heartbeatWake(_, _ time.Duration) (hal.WakeReason, error) {
	return hal.WakeHeartbeat, nil
}

func newTestManager(sys *mockSystem, sens []sensor.Device) (*Manager, *mockSink) {
	cfg := config.Config{
		SampleInterval:    time.Second,
		HeartbeatInterval: 0,
		LogLevel:          log.LevelDebug,
	}
	ms := &mockSink{name: "test"}
	man := New(sys, cfg, sens)
	man.AddSink(ms)
	return man, ms
}

// --- tests ---

func TestStep_SampleWake(t *testing.T) {
	dev := &mockSensor{
		name: "test-sensor",
		measurements: []sensor.Measurement{
			{Label: "temp", Value: "22", Unit: "C"},
		},
	}

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, []sensor.Device{dev})
	man.step()

	if !dev.initCalled {
		t.Error("sensor.Init() was not called")
	}
	if !dev.measureCalled {
		t.Error("sensor.Measure() was not called")
	}

	if len(ms.measurements) != 1 {
		t.Fatalf("sink got %d measurements, want 1", len(ms.measurements))
	}
	if ms.measurements[0].Label != "temp" {
		t.Errorf("measurement label = %q, want %q", ms.measurements[0].Label, "temp")
	}
	if ms.measDevice != "test-sensor" {
		t.Errorf("measurement device = %q, want %q", ms.measDevice, "test-sensor")
	}
}

func TestStep_HeartbeatWake(t *testing.T) {
	dev := &mockSensor{name: "test-sensor"}

	sys := &mockSystem{
		name:    "mock",
		sleepFn: heartbeatWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, []sensor.Device{dev})
	man.step()

	if dev.initCalled {
		t.Error("sensor.Init() should not be called on heartbeat wake")
	}

	if !ms.hasLog("heartbeat") {
		t.Errorf("expected heartbeat in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_SensorInitError(t *testing.T) {
	dev := &mockSensor{
		name:    "bad-sensor",
		initErr: errors.New("init failed"),
	}

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, []sensor.Device{dev})
	man.step()

	if !dev.initCalled {
		t.Error("sensor.Init() was not called")
	}
	if dev.measureCalled {
		t.Error("sensor.Measure() should not be called after init error")
	}

	if !ms.hasLog("failed to initialize") {
		t.Errorf("expected init error in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_SensorMeasureError(t *testing.T) {
	dev := &mockSensor{
		name:       "flaky-sensor",
		measureErr: errors.New("read timeout"),
	}

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, []sensor.Device{dev})
	man.step()

	if !dev.measureCalled {
		t.Error("sensor.Measure() was not called")
	}

	if !ms.hasLog("failed to measure") {
		t.Errorf("expected measure error in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_LEDCallbacks(t *testing.T) {
	var ledOnCalled, ledOffCalled bool

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, _ := newTestManager(sys, nil)
	man.OnLED(func() { ledOnCalled = true }, func() { ledOffCalled = true })
	man.step()

	if !ledOffCalled {
		t.Error("LEDOff callback was not called")
	}
	if !ledOnCalled {
		t.Error("LEDOn callback was not called")
	}
}

func TestStep_WaitForSerial(t *testing.T) {
	var waitCalled bool

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, _ := newTestManager(sys, nil)
	man.OnWaitForSerial(func() error {
		waitCalled = true
		return nil
	})
	man.step()

	if !waitCalled {
		t.Error("WaitForSerial callback was not called")
	}
}

func TestStep_WaitForSerialError(t *testing.T) {
	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, nil)
	man.OnWaitForSerial(func() error {
		return errors.New("serial timeout")
	})
	man.step()

	if !ms.hasLog("serial timeout") {
		t.Errorf("expected serial timeout error in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_ReadTimeError(t *testing.T) {
	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn: func() (time.Time, error) {
			return time.Time{}, errors.New("rtc dead")
		},
	}

	man, ms := newTestManager(sys, nil)
	man.step()

	if !ms.hasLog("rtc:") {
		t.Errorf("expected rtc error in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_SleepError(t *testing.T) {
	sys := &mockSystem{
		name: "mock",
		sleepFn: func(_, _ time.Duration) (hal.WakeReason, error) {
			return hal.WakeSample, errors.New("standby failed")
		},
		timeFn: fixedTime,
	}

	man, ms := newTestManager(sys, nil)
	man.step()

	if !ms.hasLog("sleep: standby failed") {
		t.Errorf("expected sleep error in sink logs, got: %v", ms.logEntries)
	}
}

func TestStep_NilCallbacks(t *testing.T) {
	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	// All callbacks nil -- should not panic.
	man, _ := newTestManager(sys, nil)
	man.step()
}

func TestStep_MultipleMeasurements(t *testing.T) {
	dev := &mockSensor{
		name: "multi",
		measurements: []sensor.Measurement{
			{Label: "temp", Value: "22", Unit: "C"},
			{Label: "rh", Value: "55", Unit: "%"},
		},
	}

	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, []sensor.Device{dev})
	man.step()

	if len(ms.measurements) != 2 {
		t.Fatalf("sink got %d measurements, want 2", len(ms.measurements))
	}
	if ms.measurements[1].Label != "rh" {
		t.Errorf("measurement[1].Label = %q, want %q", ms.measurements[1].Label, "rh")
	}
}

func TestStep_FlushBeforeSleep(t *testing.T) {
	sys := &mockSystem{
		name:    "mock",
		sleepFn: sampleWake,
		timeFn:  fixedTime,
	}

	man, ms := newTestManager(sys, nil)
	man.step()

	if !ms.flushCalled {
		t.Error("sink Flush() was not called before sleep")
	}
}
