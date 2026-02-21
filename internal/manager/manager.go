package manager

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/wait"
)

// system is the interface the manager needs from the hardware layer.
// It is satisfied by *hal.System and by test mocks.
type system interface {
	ReadTime() (time.Time, error)
	Sleep() ([]bool, error)
	NextWake() time.Duration
	EnableSensorRails()
}

// Group is a resolved runtime group with sensors already wired up.
// Built by the caller (main.go) from config.Group.
type Group struct {
	Name     string
	PulseLED bool
	Sensors  []sensor.Device
	Host     string
	Payload  config.Payload
}

type Manager struct {
	sys       system
	groups    []Group
	recorders []sensor.Recorder
	logger    *log.Logger
	wakeTime  time.Time
	ledOn     func()
	ledOff    func()
	petWDT    func()
	buf       [64]byte
}

func New(sys system, groups []Group, recorders []sensor.Recorder, logger *log.Logger) *Manager {
	return &Manager{
		sys:       sys,
		groups:    groups,
		recorders: recorders,
		logger:    logger,
		ledOn:     func() {},
		ledOff:    func() {},
	}
}

// EnableWatchdog sets a callback the manager calls to pet the hardware
// watchdog at strategic points.
func (m *Manager) EnableWatchdog(pet func()) {
	m.petWDT = pet
}

// SetLED sets callbacks the manager uses to pulse the LED for groups
// that have pulse_led enabled. Both callbacks must be non-nil.
func (m *Manager) SetLED(on, off func()) {
	if on == nil || off == nil {
		return
	}
	m.ledOn = on
	m.ledOff = off
}

// Run enters the main loop.
func (m *Manager) Run() {
	for {
		m.step()
	}
}

// step executes a single sleep/wake cycle.
func (m *Manager) step() {
	fired := m.doSleep()

	needsSensors := false
	anyFired := false
	wantLED := false
	for i, f := range fired {
		if !f {
			continue
		}
		anyFired = true
		if m.groups[i].PulseLED {
			wantLED = true
		}
		if len(m.groups[i].Sensors) > 0 {
			needsSensors = true
		}
	}
	if needsSensors {
		m.sys.EnableSensorRails()
	}

	for i, f := range fired {
		if !f {
			continue
		}
		m.doGroup(&m.groups[i])
	}

	if !anyFired {
		m.logger.Debug("external wake")
	}

	if wantLED {
		m.pulseLED()
	}

	m.logMem()
	runtime.GC()
}

func (m *Manager) doSleep() []bool {
	nextWake := m.sys.NextWake()
	m.logNextWake(nextWake)

	m.pet()

	if err := m.flush(); err != nil {
		m.logger.Error("flush: " + err.Error())
	}

	m.pet()

	fired, err := m.sys.Sleep()
	if err != nil {
		m.logger.Error("sleep: " + err.Error())
	}

	t, rtcErr := m.sys.ReadTime()
	if rtcErr != nil {
		t = time.Now()
	}
	m.wakeTime = t
	m.logger.SetTime(t)

	if rtcErr != nil {
		m.logger.Error("rtc: " + rtcErr.Error())
	}

	return fired
}

func (m *Manager) doGroup(g *Group) {
	m.logger.Debug("group: " + g.Name)

	for _, s := range g.Sensors {
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

		for _, r := range m.recorders {
			r.Record(m.wakeTime, s.Name(), ms)
		}
	}

	if g.Host != "" {
		// TODO: POST payload to g.Host based on g.Payload
		m.logger.Debug("payload: " + g.Host)
	}
}

func (m *Manager) pet() {
	if m.petWDT != nil {
		m.petWDT()
	}
}

func (m *Manager) pulseLED() {
	m.ledOn()
	wait.For(50 * time.Millisecond)
	m.ledOff()
	wait.For(100 * time.Millisecond)
	m.ledOn()
	wait.For(50 * time.Millisecond)
	m.ledOff()
}

func (m *Manager) logNextWake(d time.Duration) {
	b := m.buf[:0]
	b = append(b, "sleep: next wake="...)
	if d <= 0 {
		b = append(b, "external"...)
	} else {
		b = appendDuration(b, d)
	}
	m.logger.Debug(string(b))
}

func appendDuration(b []byte, d time.Duration) []byte {
	switch {
	case d < time.Minute:
		secs := (d + time.Second/2) / time.Second
		b = strconv.AppendInt(b, int64(secs), 10)
		return append(b, 's')
	case d < time.Hour:
		mins := (d + time.Minute/2) / time.Minute
		b = strconv.AppendInt(b, int64(mins), 10)
		return append(b, 'm')
	case d < 24*time.Hour:
		hrs := (d + time.Hour/2) / time.Hour
		b = strconv.AppendInt(b, int64(hrs), 10)
		return append(b, 'h')
	default:
		days := (d + 12*time.Hour) / (24 * time.Hour)
		b = strconv.AppendInt(b, int64(days), 10)
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

func (m *Manager) flush() error {
	var errs []error
	if err := m.logger.Flush(); err != nil {
		errs = append(errs, err)
	}
	for _, r := range m.recorders {
		if err := r.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
