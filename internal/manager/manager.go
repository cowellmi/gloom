package manager

import (
	"errors"
	"runtime"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
)

// sleeper is the interface the manager needs from the hardware layer.
// It is satisfied by *sleeper.Device and by test mocks.
type sleeper interface {
	Sleep(target time.Time) (time.Time, error)
	PowerOnSensorRails()
	PinFired(pin hal.Pin) bool
}

type deadline struct {
	interval time.Duration // zero means disabled
	next     time.Time
}

func (d *deadline) init(now time.Time) {
	if d.interval > 0 {
		d.next = now.Add(d.interval)
	}
}

func (d *deadline) fired(wakeTime time.Time) bool {
	if d.interval == 0 || wakeTime.Before(d.next) {
		return false
	}
	elapsed := wakeTime.Sub(d.next) / d.interval
	d.next = d.next.Add((elapsed + 1) * d.interval)
	return true
}

type Manager struct {
	sleeper        sleeper
	cfg            config.Config
	sensors        []sensor.Sensor
	recorders      []sensor.Recorder
	logger         *log.Logger
	wakeTime       time.Time
	blinkLED       func()
	petWDT         func()
	stackUsed      func() uint
	buf            [128]byte
	sampleDeadline deadline
	hbDeadline     deadline
}

func New(sleeper sleeper, cfg config.Config, sensors []sensor.Sensor, recorders []sensor.Recorder, logger *log.Logger) *Manager {
	return &Manager{
		sleeper:        sleeper,
		cfg:            cfg,
		sensors:        sensors,
		recorders:      recorders,
		logger:         logger,
		sampleDeadline: deadline{interval: cfg.Sample.Interval},
		hbDeadline:     deadline{interval: cfg.Heartbeat.Interval},
	}
}

// EnableWatchdog sets a callback the manager calls to pet the hardware
// watchdog at strategic points.
func (m *Manager) EnableWatchdog(pet func()) {
	m.petWDT = pet
}

// SetBlinkLED sets a callback the manager calls to blink the LED when
// the heartbeat fires with blink_led enabled. Pass nil to disable.
func (m *Manager) SetBlinkLED(fn func()) {
	m.blinkLED = fn
}

// SetStackMonitor sets a callback the manager calls each cycle to
// report stack usage alongside heap stats.
func (m *Manager) SetStackMonitor(used func() uint) {
	m.stackUsed = used
}

// Run enters the main loop. now is the current time read by the
// caller before Run; it seeds the first sleep's deadline calculations.
// A GC pass runs first to reclaim transient boot allocations.
func (m *Manager) Run(now time.Time) {
	runtime.GC()
	m.wakeTime = now
	m.sampleDeadline.init(now)
	m.hbDeadline.init(now)
	for {
		m.step()
	}
}

// step executes a single sleep/wake cycle.
func (m *Manager) step() {
	sampleFired, hbFired := m.doSleep()

	b := m.buf[:0]
	b = fmtbuf.Append(b, "wake:")
	needSensors := false

	if sampleFired {
		b = fmtbuf.AppendByte(b, ' ')
		b = fmtbuf.Append(b, "sample")
		if len(m.sensors) > 0 {
			needSensors = true
		}
	}
	if hbFired {
		b = fmtbuf.AppendByte(b, ' ')
		b = fmtbuf.Append(b, "heartbeat")
	}

	if sampleFired || hbFired {
		m.logger.Write(b, log.LevelDebug)
	}

	if needSensors {
		m.sleeper.PowerOnSensorRails()
	}

	if m.cfg.Heartbeat.LedPin != hal.NoPin && m.blinkLED != nil {
		m.blinkLED()
	}

	if needSensors {
		m.measureSensors()
	}

	if !sampleFired && !hbFired {
		m.logger.Debug("external wake")
	}

	m.logMem()
	runtime.GC()
}

// measureSensors measures each sample sensor and records results.
func (m *Manager) measureSensors() {
	for _, s := range m.sensors {
		m.pet()

		readings, err := s.Measure()
		if err != nil {
			m.logger.Error("failed to measure: " + s.ID() + ": " + err.Error())
			continue
		}

		for _, r := range m.recorders {
			if err := r.Record(m.wakeTime, s.ID(), readings); err != nil {
				m.logger.Error("failed to record: " + s.ID() + ": " + err.Error())
			}
		}
	}
}

func (m *Manager) doSleep() (sampleFired, hbFired bool) {
	now := m.wakeTime

	target := m.earliestDeadline()
	var nextWake time.Duration
	if !target.IsZero() {
		nextWake = target.Sub(now)
	}
	m.logNextWake(nextWake)

	m.pet()
	if err := m.flush(); err != nil {
		m.logger.Error("flush: " + err.Error())
	}
	m.pet()

	wakeTime, err := m.sleeper.Sleep(target)
	if err != nil {
		m.logger.Error("sleep: " + err.Error())
	}
	m.wakeTime = wakeTime
	m.logger.SetTime(wakeTime)

	sampleFired = m.sampleDeadline.fired(wakeTime)
	hbFired = m.hbDeadline.fired(wakeTime)
	if m.cfg.Sample.ExtPin != hal.NoPin && m.sleeper.PinFired(m.cfg.Sample.ExtPin) {
		sampleFired = true
	}

	return
}

// earliestDeadline returns the earlier of the two non-zero deadlines,
// or the zero time if neither is set.
func (m *Manager) earliestDeadline() time.Time {
	var best time.Time
	for _, t := range []time.Time{m.sampleDeadline.next, m.hbDeadline.next} {
		if t.IsZero() {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best
}

func (m *Manager) pet() {
	if m.petWDT != nil {
		m.petWDT()
	}
}

func (m *Manager) logNextWake(d time.Duration) {
	b := m.buf[:0]
	b = fmtbuf.Append(b, "sleep: next wake in ")
	if d <= 0 {
		b = fmtbuf.Append(b, "external")
	} else {
		b = appendDuration(b, d)
	}
	m.logger.Write(b, log.LevelDebug)
}

func appendDuration(b []byte, d time.Duration) []byte {
	switch {
	case d < time.Minute:
		secs := (d + time.Second/2) / time.Second
		b = fmtbuf.AppendInt(b, int64(secs), 10)
		return fmtbuf.AppendByte(b, 's')
	case d < time.Hour:
		mins := (d + time.Minute/2) / time.Minute
		b = fmtbuf.AppendInt(b, int64(mins), 10)
		return fmtbuf.AppendByte(b, 'm')
	case d < 24*time.Hour:
		hrs := (d + time.Hour/2) / time.Hour
		b = fmtbuf.AppendInt(b, int64(hrs), 10)
		return fmtbuf.AppendByte(b, 'h')
	default:
		days := (d + 12*time.Hour) / (24 * time.Hour)
		b = fmtbuf.AppendInt(b, int64(days), 10)
		return fmtbuf.AppendByte(b, 'd')
	}
}

func (m *Manager) logMem() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := m.buf[:0]
	b = fmtbuf.Append(b, "mem: heap_alloc=")
	b = fmtbuf.AppendUint(b, ms.HeapAlloc, 10)
	b = fmtbuf.AppendByte(b, 'B')
	if m.stackUsed != nil {
		b = fmtbuf.Append(b, " stack_alloc=")
		b = fmtbuf.AppendUint(b, uint64(m.stackUsed()), 10)
		b = fmtbuf.AppendByte(b, 'B')
	}
	m.logger.Write(b, log.LevelDebug)
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
