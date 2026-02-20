package manager

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/wait"
)

// system is the interface the manager needs from the hardware layer.
// It is satisfied by *hal.System and by test mocks.
type system interface {
	Identifier() string
	ReadTime() (time.Time, error)
	Sleep() (hal.WakeReason, error)
	NextWake() (sample, heartbeat time.Duration)
}

type Manager struct {
	sys        system
	cfg        config.Config
	sensors    []sensor.Device
	recorders  []sensor.Recorder
	logger     *log.Logger
	wakeTime   time.Time
	ledEnabled bool
	ledOn      func()
	ledOff     func()
	petWDT     func()
	buf        [64]byte // scratch buffer for formatting text.
}

func New(sys system, cfg config.Config, devices []sensor.Device, logger *log.Logger) *Manager {
	return &Manager{
		sys:     sys,
		cfg:     cfg,
		sensors: devices,
		logger:  logger,
		ledOn:   func() {}, // default no-op functions
		ledOff:  func() {},
	}
}

// AddRecorder registers a recorder for measurement output. Recorders
// are called in registration order on each sample cycle.
func (m *Manager) AddRecorder(r sensor.Recorder) {
	m.recorders = append(m.recorders, r)
}

// EnableWatchdog sets a callback the manager calls to pet the hardware
// watchdog at strategic points (before flush, after log bursts, etc.)
// to prevent resets during long SD card operations.
func (m *Manager) EnableWatchdog(pet func()) {
	m.petWDT = pet
}

// EnableLED sets callbacks the manager uses to pulse the LED during
// heartbeat wakes. Both callbacks must be non-nil.
func (m *Manager) EnableLED(on, off func()) {
	if on == nil || off == nil {
		return
	}
	m.ledEnabled = true
	m.ledOn = on
	m.ledOff = off
}

// Run enters the main loop. The loop body lives in step() so
// tests can execute a single iteration without blocking forever.
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
	if reason == hal.WakeExternal {
		m.logger.Debug("external wake")
	}

	m.logMem()
	runtime.GC()
}

func (m *Manager) doSleep() hal.WakeReason {
	nextSample, nextHeartbeat := m.sys.NextWake()
	m.logNextWake(nextSample, nextHeartbeat)

	m.pet()

	// Flush all outputs before going to sleep.
	if err := m.flush(); err != nil {
		m.logger.Error("flush: " + err.Error())
	}

	m.pet()

	// Put system to sleep. Execution halts here until wake from sleep.
	reason, err := m.sys.Sleep()
	if err != nil {
		m.logger.Error("sleep: " + err.Error())
	}
	// Resume execution after wake from sleep.

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

func (m *Manager) doSample() {
	m.logger.Debug("sample")
	for _, s := range m.sensors {
		m.pet()

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
			r.Record(m.wakeTime, s.Name(), ms)
		}
	}
}

func (m *Manager) doHeartbeat() {
	m.logger.Debug("heartbeat")
	if m.ledEnabled {
		m.pulseLED()
	}
	// TODO: transmit keep-alive message
}

// pet resets the watchdog countdown if a callback has been registered.
func (m *Manager) pet() {
	if m.petWDT != nil {
		m.petWDT()
	}
}

// Pulse LED on/off, on/off
func (m *Manager) pulseLED() {
	m.ledOn()
	wait.For(50 * time.Millisecond)
	m.ledOff()
	wait.For(100 * time.Millisecond)
	m.ledOn()
	wait.For(50 * time.Millisecond)
	m.ledOff()
}

func (m *Manager) logNextWake(nextSample, nextHeartbeat time.Duration) {
	b := m.buf[:0]
	b = append(b, "sleep: next wake: sample="...)
	if m.cfg.SampleInterval <= 0 {
		b = append(b, "disabled"...)
	} else {
		b = appendDuration(b, nextSample)
	}
	b = append(b, " heartbeat="...)
	if m.cfg.HeartbeatInterval <= 0 {
		b = append(b, "disabled"...)
	} else {
		b = appendDuration(b, nextHeartbeat)
	}
	m.logger.Debug(string(b))
}

// appendDuration appends a human-readable duration to b using the
// largest whole unit that fits: days, hours, minutes, or seconds.
func appendDuration(b []byte, d time.Duration) []byte {
	switch {
	case d < time.Minute:
		b = strconv.AppendInt(b, int64(d/time.Second), 10)
		return append(b, 's')
	case d < time.Hour:
		b = strconv.AppendInt(b, int64(d/time.Minute), 10)
		return append(b, 'm')
	case d < 24*time.Hour:
		b = strconv.AppendInt(b, int64(d/time.Hour), 10)
		return append(b, 'h')
	default:
		b = strconv.AppendInt(b, int64(d/(24*time.Hour)), 10)
		return append(b, 'd')
	}
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
func (m *Manager) flush() error {
	var errs []error
	err := m.logger.Flush()
	if err != nil {
		errs = append(errs, err)
	}
	for _, r := range m.recorders {
		err := r.Flush()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
