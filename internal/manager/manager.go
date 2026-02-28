package manager

import (
	"errors"
	"runtime"
	"sort"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink"
)

type sleeper interface {
	Sleep(target time.Time) (time.Time, error)
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

// Group is a named schedule group with its own deadline and config.
// Use BuildGroup to construct one from a name and config.Group.
type Group struct {
	name     string
	cfg      config.Group
	deadline deadline
}

// BuildGroup constructs a Group from the given name and config.Group.
func BuildGroup(name string, cfg config.Group) Group {
	return Group{
		name:     name,
		cfg:      cfg,
		deadline: deadline{interval: cfg.Interval},
	}
}

type Manager struct {
	sleeper   sleeper
	rails     hal.Rails
	led       hal.LED
	groups    []Group
	sensors   map[string]sensor.Sensor
	dataSinks []sink.DataSink
	logger    *log.Logger
	wakeTime  time.Time
	petWDT    func()
	stackUsed func() uint
	buf       [128]byte
}

// New creates a Manager from a map of config groups.
// Group names are sorted alphabetically for stable bit assignments across runs.
func New(sleeper sleeper, rails hal.Rails, led hal.LED,
	groups map[string]config.Group, sensors map[string]sensor.Sensor,
	dataSinks []sink.DataSink, logger *log.Logger) *Manager {

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	gs := make([]Group, len(names))
	for i, name := range names {
		gs[i] = BuildGroup(name, groups[name])
	}

	return &Manager{
		sleeper:   sleeper,
		rails:     rails,
		led:       led,
		groups:    gs,
		sensors:   sensors,
		dataSinks: dataSinks,
		logger:    logger,
	}
}

// EnableWatchdog sets a callback the manager calls to pet the hardware
// watchdog at strategic points.
func (m *Manager) EnableWatchdog(pet func()) {
	m.petWDT = pet
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
	for i := range m.groups {
		m.groups[i].deadline.init(now)
	}
	for {
		m.step()
	}
}

// step executes a single sleep/wake cycle.
func (m *Manager) step() {
	fired := m.doSleep()

	b := m.buf[:0]
	b = fmtbuf.Append(b, "wake:")
	needSensors := false
	pulseLED := false

	for i := range m.groups {
		if fired&(1<<uint(i)) == 0 {
			continue
		}
		g := &m.groups[i]
		b = fmtbuf.AppendByte(b, ' ')
		b = fmtbuf.Append(b, g.name)
		if len(g.cfg.Sensors) > 0 {
			needSensors = true
		}
		if g.cfg.PulseLED {
			pulseLED = true
		}
	}

	if fired != 0 {
		m.logger.Log(config.LogLevelDebug, string(b))
	}

	if pulseLED {
		m.led.Blink()
	}

	if needSensors {
		m.rails.Power(hal.RailsFull)
		m.measureFiredSensors(fired)
		m.rails.Power(hal.RailsCore)
	}

	if fired == 0 {
		m.logger.Log(config.LogLevelDebug, "external wake")
	}

	m.logMem()
	runtime.GC()
}

// measureFiredSensors measures the deduplicated union of sensor IDs from
// all fired groups. Each unique sensor is measured exactly once.
// Uses O(n²) deduplication with a fixed-size stack buffer — no heap allocation.
func (m *Manager) measureFiredSensors(fired uint32) {
	var ids [32]string
	n := 0

	for i := range m.groups {
		if fired&(1<<uint(i)) == 0 {
			continue // group did not fire
		}
		for _, id := range m.groups[i].cfg.Sensors {
			dup := false
			for j := 0; j < n; j++ {
				if ids[j] == id {
					dup = true
					break
				}
			}
			if !dup && n < len(ids) {
				ids[n] = id
				n++
			}
		}
	}

	for _, id := range ids[:n] {
		s, ok := m.sensors[id]
		if !ok {
			continue
		}
		m.pet()

		readings, err := s.Measure()
		if err != nil {
			m.logger.LogError(config.LogLevelError, err, "measure: "+id+": ")
			continue
		}

		for _, ds := range m.dataSinks {
			if err := ds.Data(m.wakeTime, id, readings); err != nil {
				m.logger.LogError(config.LogLevelError, err, "record: "+id+": ")
			}
		}
	}
}

func (m *Manager) doSleep() uint32 {
	now := m.wakeTime

	target := m.earliestDeadline()
	var nextWake time.Duration
	if !target.IsZero() {
		nextWake = target.Sub(now)
	}
	m.logNextWake(nextWake)

	m.pet()
	if err := m.flush(); err != nil {
		m.logger.LogError(config.LogLevelError, err, "flush: ")
	}
	m.pet()

	wakeTime, err := m.sleeper.Sleep(target)
	if err != nil {
		m.logger.LogError(config.LogLevelError, err, "sleep: ")
	}
	m.wakeTime = wakeTime
	m.logger.SetTime(wakeTime)

	var fired uint32
	for i := range m.groups {
		g := &m.groups[i]
		if g.deadline.fired(wakeTime) {
			fired |= 1 << uint(i)
		}
		for _, pin := range g.cfg.InterruptPins {
			if m.sleeper.PinFired(pin) {
				fired |= 1 << uint(i)
				break
			}
		}
	}

	return fired
}

// earliestDeadline returns the earliest non-zero deadline across all groups,
// or the zero time if no group has a periodic interval.
func (m *Manager) earliestDeadline() time.Time {
	var best time.Time
	for i := range m.groups {
		t := m.groups[i].deadline.next
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
		b = fmtbuf.AppendDuration(b, d)
	}
	m.logger.Log(config.LogLevelDebug, string(b))
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
	m.logger.Log(config.LogLevelDebug, string(b))
}

func (m *Manager) flush() error {
	var errs []error
	if err := m.logger.Flush(); err != nil {
		errs = append(errs, err)
	}
	for _, ds := range m.dataSinks {
		if err := ds.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
