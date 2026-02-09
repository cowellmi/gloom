package manager

import (
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// Size of the shared scratch buffer used by recorders for formatting.
// 512 bytes comfortably fits any single CSV row or JSON payload
// without heap allocation.
const recorderBufSize = 512

type Manager struct {
	sys         hal.Platform
	cfg         config.Config
	sensors     []sensor.Device
	recorders   []sensor.Recorder
	logger      *log.Logger
	buf         [recorderBufSize]byte
	wakeTime    time.Time
	serialReady func() bool
	ledEnabled  bool
	ledOn       func()
	ledOff      func()
}

func New(sys hal.Platform, cfg config.Config, devices []sensor.Device, logger *log.Logger) *Manager {
	return &Manager{
		sys:     sys,
		cfg:     cfg,
		sensors: devices,
		logger:  logger,
	}
}

// AddRecorder registers a recorder for measurement output. Recorders
// are called in registration order on each sample cycle.
func (m *Manager) AddRecorder(r sensor.Recorder) {
	m.recorders = append(m.recorders, r)
}

// EnableLED sets callbacks invoked when the manager turns the LED off
// (before sleep) and on (after wake). Nil by default (LED ignored).
func (m *Manager) EnableLED(on, off func()) {
	if on == nil || off == nil {
		return
	}
	m.ledEnabled = true
	m.ledOn = on
	m.ledOff = off
}

// OnSerialReady sets a polling function that returns true when a
// serial connection is available (e.g. USB DTR is asserted). The
// manager polls this after each wake, pulsing the LED while waiting.
// Nil by default (skipped).
func (m *Manager) OnSerialReady(fn func() bool) {
	m.serialReady = fn
}

func (m *Manager) Run() {
	m.logger.Debug("platform: " + m.sys.Identifier())
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

	m.logMem()

	// Force GC to collect per-cycle allocations.
	runtime.GC()
}

func (m *Manager) doSleep() hal.WakeReason {
	sampleInterval := m.cfg.SampleInterval.String()
	if m.cfg.SampleInterval <= 0 {
		sampleInterval = "disabled"
	}
	heartbeatInterval := m.cfg.HeartbeatInterval.String()
	if m.cfg.HeartbeatInterval <= 0 {
		heartbeatInterval = "disabled"
	}
	m.logger.Debug("sleep: sample=" + sampleInterval + " heartbeat=" + heartbeatInterval)

	// Flush all outputs before powering down peripherals and sleeping.
	m.flush()

	if m.ledEnabled {
		m.ledOff()
	}

	// Put system to sleep. Execution halts here until wake from sleep.
	reason, err := m.sys.Sleep(m.cfg.SampleInterval, m.cfg.HeartbeatInterval)
	if err != nil {
		m.logger.Error("sleep: " + err.Error())
	}
	// Resume execution after wake from sleep.

	// Wait for serial connection, pulsing LED to signal "waiting."
	if m.serialReady != nil {
		m.waitForSerial()
	}

	if m.ledEnabled {
		m.ledOn()
	}

	// Update wake time and push it to the logger so all subsequent
	// log entries in this cycle carry the correct timestamp.
	t, rtcErr := m.sys.ReadTime()
	if rtcErr != nil {
		// Fallback to system clock.
		t = time.Now()
	}
	m.wakeTime = t
	m.logger.SetTime(t)

	if rtcErr != nil {
		m.logger.Error("rtc: " + rtcErr.Error())
	}

	return reason
}

const serialPollInterval = 100 * time.Millisecond

// serialSettleDelay is the pause after standby wake to let the host
// re-enumerate USB before polling DTR.
// This needs to be a var so we can disable it during tests.
var serialSettleDelay = time.Second

// waitForSerial polls serialReady. Times out after cfg.MaxWaitForSerial.
func (m *Manager) waitForSerial() {
	time.Sleep(serialSettleDelay)

	var waited time.Duration
	for !m.serialReady() {
		if m.cfg.MaxWaitForSerial > 0 && waited >= m.cfg.MaxWaitForSerial {
			m.logger.Warn("wait for serial timed out")
			return
		}
		time.Sleep(serialPollInterval)
		waited += serialPollInterval
	}
}

func (m *Manager) doSample() {
	for _, s := range m.sensors {
		if err := s.Init(); err != nil {
			m.logger.Error("failed to initialize: " + s.Name() + ": " + err.Error())
			continue
		}

		ms, err := s.Measure()
		if err != nil {
			m.logger.Error("failed to measure: " + s.Name() + ": " + err.Error())
			continue
		}

		// Fan out structured measurements to all recorders.
		// Each recorder formats as appropriate (text for serial, CSV for SD, etc).
		for _, r := range m.recorders {
			r.Record(m.buf[:0], m.wakeTime, s.Name(), ms)
		}
	}
}

func (m *Manager) doHeartbeat() {
	// Double-pulse like a heartbeat: thump-thump.
	if m.ledEnabled {
		m.ledOn()
		time.Sleep(100 * time.Millisecond)
		m.ledOff()
		time.Sleep(100 * time.Millisecond)
		m.ledOn()
		time.Sleep(100 * time.Millisecond)
		m.ledOff()
	}

	m.logger.Debug("heartbeat")
	// TODO: transmit keep-alive message
}

func (m *Manager) logMem() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := m.buf[:0]
	b = append(b, "mem: heap_alloc="...)
	b = strconv.AppendUint(b, ms.HeapAlloc/1024, 10)
	b = append(b, "kb heap_sys="...)
	b = strconv.AppendUint(b, ms.HeapSys/1024, 10)
	b = append(b, "kb"...)
	m.logger.Debug(string(b))
}

// flush flushes the logger sinks and all recorders. Called before
// sleep so buffered data (SD writes, network payloads) is committed
// before the MCU enters standby.
func (m *Manager) flush() {
	m.logger.Flush()
	for _, r := range m.recorders {
		r.Flush()
	}
}
