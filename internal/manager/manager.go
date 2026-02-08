package manager

import (
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink"
)

type Manager struct {
	sys           hal.Platform
	cfg           config.Config
	sensors       []sensor.Device
	sinks         []sink.Sink
	wakeTime      time.Time
	waitForSerial func() error
	ledOn         func()
	ledOff        func()
}

func New(sys hal.Platform, cfg config.Config, devices []sensor.Device) *Manager {
	return &Manager{
		sys:     sys,
		cfg:     cfg,
		sensors: devices,
	}
}

// AddSink registers a sink for measurement and log output. Sinks are
// called in registration order on each sample and sleep cycle.
func (m *Manager) AddSink(s sink.Sink) {
	m.sinks = append(m.sinks, s)
}

// OnLED sets callbacks invoked when the manager turns the LED off
// (before sleep) and on (after wake). Nil by default (LED ignored).
func (m *Manager) OnLED(on, off func()) {
	m.ledOn = on
	m.ledOff = off
}

// OnWaitForSerial sets a callback invoked after each wake to wait
// for a serial connection. Nil by default (skipped).
func (m *Manager) OnWaitForSerial(fn func() error) {
	m.waitForSerial = fn
}

func (m *Manager) Run() {
	for {
		m.step()
	}
}

// step executes a single sleep/wake cycle: sleep, then sample
// and/or heartbeat depending on the wake reason.
func (m *Manager) step() {
	reason := m.doSleep()

	if reason&hal.WakeSample != 0 {
		m.doSample()
	}
	if reason&hal.WakeHeartbeat != 0 {
		m.doHeartbeat()
	}

	if m.cfg.LogLevel == log.LevelDebug {
		m.slog(log.LevelDebug, "platform: "+m.sys.Identifier())
		m.logMem()
	}

	// Force GC to collect per-cycle allocations.
	runtime.GC()
}

func (m *Manager) doSleep() hal.WakeReason {
	if m.cfg.LogLevel == log.LevelDebug {
		sampleInterval := m.cfg.SampleInterval.String()
		if m.cfg.SampleInterval == 0 {
			sampleInterval = "disabled"
		}
		heartbeatInterval := m.cfg.HeartbeatInterval.String()
		if m.cfg.HeartbeatInterval == 0 {
			heartbeatInterval = "disabled"
		}
		m.slog(log.LevelDebug, "sleep: sample="+sampleInterval+" heartbeat="+heartbeatInterval)
	}

	// Flush all sinks before powering down peripherals and sleeping.
	m.flushSinks()

	if m.ledOff != nil {
		m.ledOff()
	}

	// Put system to sleep. Execution halts here until wake from sleep.
	reason, err := m.sys.Sleep(m.cfg.SampleInterval, m.cfg.HeartbeatInterval)
	if err != nil {
		// We could handle any specific errors here but for now just log.
		m.slog(log.LevelError, "sleep: "+err.Error())
	}
	// Resume execution after wake from sleep.

	if m.ledOn != nil {
		m.ledOn()
	}

	// We check this here because this can be a long running
	// process and we cache the wake time immediately after.
	if m.waitForSerial != nil {
		err := m.waitForSerial()
		if err != nil {
			m.slog(log.LevelError, err.Error())
		}
	}

	// Update wake time. A good thing to note explicitly: doing
	// this makes the timestamp for sensors reading less accurate
	// since we are caching the time here instead of immediately
	// before each sensor.Measure() execution. In practice it
	// shouldn't be much of a difference.
	t, rtcErr := m.sys.ReadTime()
	if rtcErr != nil {
		// Fallback to system clock.
		t = time.Now()
	}
	m.wakeTime = t
	if rtcErr != nil {
		m.slog(log.LevelError, "rtc: "+rtcErr.Error())
	}

	return reason
}

func (m *Manager) doSample() {
	for _, s := range m.sensors {
		if err := s.Init(); err != nil {
			m.slog(log.LevelError, "failed to initialize: "+s.Name()+": "+err.Error())
			continue
		}

		ms, err := s.Measure()
		if err != nil {
			m.slog(log.LevelError, "failed to measure: "+s.Name()+": "+err.Error())
			continue
		}

		// Fan out structured measurements to all sinks.
		// Each sink formats as appropriate (text for serial, CSV for SD, etc).
		m.writeMeasurements(s.Name(), ms)
	}
}

func (m *Manager) doHeartbeat() {
	m.slog(log.LevelDebug, "heartbeat")
	// TODO: transmit keep-alive message
}

// slog writes a log entry to all sinks, filtered by the configured
// minimum log level.
func (m *Manager) slog(level log.Level, msg string) {
	if level < m.cfg.LogLevel {
		return
	}
	for _, s := range m.sinks {
		// Sink errors are silently ignored. Sinks self-disable
		// on persistent write failures.
		s.WriteLog(m.wakeTime, level, msg)
	}
}

// writeMeasurements fans out a measurement batch to all registered sinks.
func (m *Manager) writeMeasurements(device string, ms []sensor.Measurement) {
	for _, s := range m.sinks {
		s.WriteMeasurements(m.wakeTime, device, ms)
	}
}

// flushSinks flushes all registered sinks. Called before sleep so
// buffered data (SD writes, network payloads) is committed before
// the MCU enters standby.
func (m *Manager) flushSinks() {
	for _, s := range m.sinks {
		s.Flush()
	}
}

func formatBytes(b uint64) string {
	whole := b / 1024
	return strconv.FormatUint(whole, 10)
}

func (m *Manager) logMem() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.slog(log.LevelDebug, "mem: heap_alloc="+formatBytes(ms.HeapAlloc)+"kb"+
		" heap_sys="+formatBytes(ms.HeapSys)+"kb")
}
