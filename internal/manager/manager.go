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
// It is satisfied by *sleeper.Sleeper and by test mocks.
// Uses uint8 for pin numbers so Manager does not import hal.
type system interface {
	ReadTime() (time.Time, error)
	Sleep(target time.Time) (time.Time, error)
	PowerOnSensorRails()
	PinFired(pin uint8) bool
}

// Group is a resolved runtime group with sensors already wired up.
// Built by the caller (main.go) from config.Group. Sensor instances
// may be shared across groups — the manager deduplicates measurements.
type Group struct {
	Name     string
	Interval time.Duration
	PulseLED bool
	Sensors  []sensor.Device
	Host     string
	Payload  config.Payload
}

// extPin associates an external interrupt pin with a group slot.
type extPin struct {
	pin  uint8
	slot int
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
	stackFree func() uint
	buf       [64]byte

	// Deduplicated sensor list built at construction. measured is a
	// parallel slice reset each cycle to track which sensors have
	// already been init'd + measured, avoiding duplicate I2C traffic
	// when multiple fired groups share a sensor.
	allSensors []sensor.Device
	measured   []bool

	// Deadline tracking (parallel to groups).
	deadlines []time.Time // zero until first initialized
	fired     []bool      // reused each cycle

	// External interrupt pins registered by main.go before Run.
	extPins []extPin
}

func New(sys system, groups []Group, recorders []sensor.Recorder, logger *log.Logger) *Manager {
	var allSensors []sensor.Device
	for _, g := range groups {
		for _, s := range g.Sensors {
			found := false
			for _, existing := range allSensors {
				if existing == s {
					found = true
					break
				}
			}
			if !found {
				allSensors = append(allSensors, s)
			}
		}
	}

	n := len(groups)
	return &Manager{
		sys:        sys,
		groups:     groups,
		recorders:  recorders,
		logger:     logger,
		ledOn:      func() {},
		ledOff:     func() {},
		allSensors: allSensors,
		measured:   make([]bool, len(allSensors)),
		deadlines:  make([]time.Time, n),
		fired:      make([]bool, n),
	}
}

// RegisterExternalPin records that pin (as uint8) maps to group slot.
// Called from main.go before Run. Manager checks sys.PinFired(pin)
// after each wake to determine whether to fire that group.
func (m *Manager) RegisterExternalPin(pin uint8, slot int) {
	m.extPins = append(m.extPins, extPin{pin: pin, slot: slot})
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

// SetStackMonitor sets a callback the manager calls each cycle to
// report remaining stack headroom alongside heap stats.
func (m *Manager) SetStackMonitor(fn func() uint) {
	m.stackFree = fn
}

// Run enters the main loop. A GC pass runs first to reclaim
// transient boot allocations before the first sleep.
func (m *Manager) Run() {
	runtime.GC()
	for {
		m.step()
	}
}

// step executes a single sleep/wake cycle.
func (m *Manager) step() {
	fired := m.doSleep()

	// Single pass: gather flags and build the "wake:" log message.
	needsSensors := false
	anyFired := false
	wantLED := false
	b := m.buf[:0]
	b = append(b, "wake:"...)

	for i, f := range fired {
		if !f {
			continue
		}
		anyFired = true
		b = append(b, ' ')
		b = append(b, m.groups[i].Name...)
		if m.groups[i].PulseLED {
			wantLED = true
		}
		if len(m.groups[i].Sensors) > 0 {
			needsSensors = true
		}
	}

	if needsSensors {
		m.sys.PowerOnSensorRails()
	}

	if anyFired {
		m.logger.Debug(string(b))
	}

	// Measure each unique sensor once across all fired groups.
	if needsSensors {
		m.measureSensors(fired)
	}

	// Per-group host/payload dispatch.
	for i, f := range fired {
		if !f {
			continue
		}
		g := &m.groups[i]
		if g.Host != "" {
			// TODO: POST payload to g.Host based on g.Payload
			m.logger.Debug("payload: " + g.Host)
		}
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

// measureSensors inits and measures each unique sensor that belongs to
// at least one fired group, recording results once to all recorders.
func (m *Manager) measureSensors(fired []bool) {
	for i := range m.measured {
		m.measured[i] = false
	}

	for i, f := range fired {
		if !f {
			continue
		}
		for _, s := range m.groups[i].Sensors {
			idx := m.sensorIndex(s)
			if idx < 0 {
				m.logger.Error("sensor not in pool: " + s.Name())
				continue
			}
			if m.measured[idx] {
				continue
			}
			m.measured[idx] = true

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
	}
}

func (m *Manager) sensorIndex(s sensor.Device) int {
	for i, existing := range m.allSensors {
		if existing == s {
			return i
		}
	}
	return -1
}

func (m *Manager) doSleep() []bool {
	// 1. Read time for deadline initialization and next-wake logging.
	now, rtcErr := m.sys.ReadTime()
	if rtcErr != nil {
		now = time.Now()
	}

	// 2. Initialize zero deadlines on first call.
	for i, g := range m.groups {
		if g.Interval > 0 && m.deadlines[i].IsZero() {
			m.deadlines[i] = now.Add(g.Interval)
		}
	}

	// 3. Compute the earliest deadline and log the upcoming sleep.
	target, _ := m.earliestDeadline()
	var nextWake time.Duration
	if !target.IsZero() {
		nextWake = target.Sub(now)
	}
	m.logNextWake(nextWake)

	// 4. Flush before sleeping.
	m.pet()
	if err := m.flush(); err != nil {
		m.logger.Error("flush: " + err.Error())
	}
	m.pet()

	// 5. Sleep — wakeTime comes back directly; no additional ReadTime call.
	wakeTime, sleepErr := m.sys.Sleep(target)
	m.wakeTime = wakeTime
	m.logger.SetTime(wakeTime)

	// 6. Log errors.
	if sleepErr != nil {
		m.logger.Error("sleep: " + sleepErr.Error())
	}
	if rtcErr != nil {
		m.logger.Error("rtc: " + rtcErr.Error())
	}

	// 7. Resolve which deadline slots fired.
	for i := range m.fired {
		m.fired[i] = false
	}
	for i, g := range m.groups {
		if g.Interval > 0 && !m.deadlines[i].IsZero() && !wakeTime.Before(m.deadlines[i]) {
			m.fired[i] = true
			for !wakeTime.Before(m.deadlines[i]) {
				m.deadlines[i] = m.deadlines[i].Add(g.Interval)
			}
		}
	}

	// 8. Resolve ext-pin slots. Checked independently of deadlines
	// since both can trigger during the same sleep cycle.
	for _, ep := range m.extPins {
		if m.sys.PinFired(ep.pin) {
			m.fired[ep.slot] = true
		}
	}

	return m.fired
}

// earliestDeadline returns the earliest non-zero deadline entry and
// whether any interval-based deadlines exist at all.
func (m *Manager) earliestDeadline() (time.Time, bool) {
	var best time.Time
	for i, g := range m.groups {
		if g.Interval <= 0 {
			continue
		}
		t := m.deadlines[i]
		if t.IsZero() {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best, !best.IsZero()
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
	b = append(b, "sleep: next wake in "...)
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
	b = append(b, "kb"...)
	if m.stackFree != nil {
		b = append(b, " stack_free="...)
		b = strconv.AppendUint(b, uint64(m.stackFree()), 10)
		b = append(b, 'b')
	}
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
