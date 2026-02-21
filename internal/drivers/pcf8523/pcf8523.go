// Package pcf8523 implements hal.RTC for the NXP PCF8523 real-time clock
// found on the Adalogger FeatherWing. It wraps the TinyGo driver for
// basic time and power management, and adds countdown timer A logic
// for wake interrupts. Lives under drivers/ to decouple peripheral
// implementations from the HAL interfaces they satisfy.
//
// Wake interrupts use countdown timer A rather than the alarm because
// the alarm only has minute-level granularity while the timer supports
// sub-second precision and maps directly to durations.
package pcf8523

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"

	"tinygo.org/x/drivers"
	driver "tinygo.org/x/drivers/pcf8523"
)

// Name is the human-readable identifier for this RTC.
const Name = "PCF8523"

// Register addresses not exposed by the upstream driver.
const (
	regControl2  = 0x01
	regSeconds   = 0x03
	regTmrClkout = 0x0F
	regTmrAFreq  = 0x10
	regTmrAReg   = 0x11
)

// Control_2 bits.
const (
	bitCTAF  = 1 << 6 // countdown timer A flag
	bitCTAIE = 1 << 1 // countdown timer A interrupt enable
)

// Control_3 bits and masks (for post-reset verification).
const (
	maskPM = 0x07 << 5 // PM[2:0] in bits 7:5
)

// Tmr_CLKOUT_ctrl bits and masks.
const (
	maskCOF = 0x07 << 3 // COF[2:0] in bits 5:3
	cofOff  = 0x07 << 3 // COF = 111: CLKOUT disabled
	maskTAC = 0x03 << 1 // TAC[1:0] in bits 2:1
	tacCD   = 0x01 << 1 // TAC = 01: countdown timer mode
)

// Timer A source clock frequencies (TAQ[2:0] in regTmrAFreq).
const (
	taq4096Hz = 0x00 // 4096 Hz, max 255/4096 ≈ 62ms
	taq64Hz   = 0x01 // 64 Hz,   max 255/64  ≈ 3.98s
	taq1Hz    = 0x02 // 1 Hz,    max 255s     ≈ 4.25min
	taq1_60Hz = 0x03 // 1/60 Hz, max 15300s   ≈ 4.25hr
)

// timerClock describes a countdown timer source clock.
type timerClock struct {
	taq       uint8         // register value for TAQ[2:0]
	tickNanos int64         // duration of one tick in nanoseconds
	maxDur    time.Duration // maximum representable duration (255 ticks)
}

// clocks ordered finest-first so the loop picks the finest clock whose
// count fits in a single byte, giving the best timing resolution.
var clocks = [...]timerClock{
	{taq4096Hz, int64(time.Second / 4096), 255 * time.Second / 4096},
	{taq64Hz, int64(time.Second / 64), 255 * time.Second / 64},
	{taq1Hz, int64(time.Second), 255 * time.Second},
	{taq1_60Hz, int64(60 * time.Second), 255 * 60 * time.Second},
}

var (
	errNotFound      = errors.New("pcf8523: not found")
	errDurationRange = errors.New("pcf8523: duration out of timer range")
	errProbeRetry    = errors.New("pcf8523: communication timed out")
)

const (
	probeRetries    = 5
	probeRetryDelay = 500 * time.Millisecond
)

// RTC wraps the TinyGo PCF8523 driver and satisfies hal.RTC.
type RTC struct {
	dev     *driver.Device
	bus     drivers.I2C
	addr    uint16
	wakePin uint8
}

// compile-time check
var _ hal.RTC = (*RTC)(nil)

// Probe detects a PCF8523 on the given I2C bus. wakePin is the GPIO
// pin connected to the PCF8523 INT1/CLKOUT output. The bus must
// already be configured.
//
// Probe issues a software reset, verifies the post-reset register
// state, then configures the chip for battery switch-over and
// interrupt-based wake.
func Probe(bus drivers.I2C, wakePin uint8) (*RTC, error) {
	dev := driver.New(bus)
	r := &RTC{
		dev:     &dev,
		bus:     bus,
		addr:    uint16(dev.Address),
		wakePin: wakePin,
	}

	// Retry loop — the chip may need time after power-on.
	var lastErr error
	for attempt := 0; attempt < probeRetries; attempt++ {
		if attempt > 0 {
			wait.For(probeRetryDelay)
		}
		lastErr = r.dev.Reset()
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, errProbeRetry
	}

	// Allow reset to settle.
	wait.For(2 * time.Millisecond)

	// Verify post-reset state. After reset, Control_3 bits 7:5
	// (PM[2:0]) are 111 = 0xE0. On a DS3231, register 0x02 is
	// the hours register which won't have this pattern.
	ctrl3, err := r.read(0x02) // regControl3
	if err != nil {
		return nil, errNotFound
	}
	if ctrl3&maskPM != maskPM {
		return nil, errNotFound
	}

	if err := r.configure(); err != nil {
		return nil, err
	}

	return r, nil
}

// configure sets up the PCF8523 for field use after reset.
func (r *RTC) configure() error {
	// Enable battery switch-over in standard mode with battery
	// low detection via the upstream driver.
	if err := r.dev.SetPowerManagement(driver.PowerManagement_SwitchOver_ModeStandard_LowDetection); err != nil {
		return errors.New("pcf8523: configure power management: " + err.Error())
	}

	// Disable CLKOUT so the INT1 pin is available for timer
	// interrupts. Read-modify-write to preserve other bits.
	clkout, err := r.read(regTmrClkout)
	if err != nil {
		return errors.New("pcf8523: read tmr_clkout: " + err.Error())
	}
	clkout = (clkout &^ maskCOF) | cofOff // COF = 111
	clkout = clkout &^ maskTAC            // TAC = 00 (timer A off)
	if err := r.write(regTmrClkout, clkout); err != nil {
		return errors.New("pcf8523: configure tmr_clkout: " + err.Error())
	}

	// Clear the oscillator-stopped flag (OS) in the seconds
	// register. Read, mask off bit 7, write back.
	sec, err := r.read(regSeconds)
	if err != nil {
		return errors.New("pcf8523: read seconds: " + err.Error())
	}
	if err := r.write(regSeconds, sec&0x7F); err != nil {
		return errors.New("pcf8523: clear OS flag: " + err.Error())
	}

	// Clear any stale timer A interrupt flag.
	return r.clearTimerFlag()
}

func (r *RTC) Identifier() string { return Name }

// ReadTime returns the current date and time from the PCF8523.
func (r *RTC) ReadTime() (time.Time, error) {
	return r.dev.ReadTime()
}

// SetTime writes the given time to the PCF8523.
func (r *RTC) SetTime(t time.Time) error {
	return r.dev.SetTime(t)
}

// SetWake programs countdown timer A to fire an interrupt on INT1
// at the given target time. Reads the current RTC time to compute
// the duration.
func (r *RTC) SetWake(target time.Time) error {
	now, err := r.ReadTime()
	if err != nil {
		return err
	}

	dur := target.Sub(now)
	if dur <= 0 {
		dur = 1 // fire immediately
	}

	taq, count, err := pickClock(dur)
	if err != nil {
		return err
	}

	// Set source clock frequency.
	if err := r.write(regTmrAFreq, taq); err != nil {
		return errors.New("pcf8523: set timer freq: " + err.Error())
	}

	// Set countdown value.
	if err := r.write(regTmrAReg, count); err != nil {
		return errors.New("pcf8523: set timer count: " + err.Error())
	}

	// Enable countdown timer A (TAC = 01) and keep CLKOUT off.
	clkout, err := r.read(regTmrClkout)
	if err != nil {
		return errors.New("pcf8523: read tmr_clkout: " + err.Error())
	}
	clkout = (clkout &^ maskTAC) | tacCD
	if err := r.write(regTmrClkout, clkout); err != nil {
		return errors.New("pcf8523: enable timer A: " + err.Error())
	}

	// Enable countdown timer A interrupt (CTAIE).
	ctrl2, err := r.read(regControl2)
	if err != nil {
		return errors.New("pcf8523: read control_2: " + err.Error())
	}
	ctrl2 |= bitCTAIE
	if err := r.write(regControl2, ctrl2); err != nil {
		return errors.New("pcf8523: enable CTAIE: " + err.Error())
	}

	return nil
}

// ClearWake clears the countdown timer A interrupt flag and disables
// the timer so the INT1 pin releases.
func (r *RTC) ClearWake() error {
	// Disable timer A (TAC = 00).
	clkout, err := r.read(regTmrClkout)
	if err != nil {
		return errors.New("pcf8523: read tmr_clkout: " + err.Error())
	}
	clkout &^= maskTAC
	if err := r.write(regTmrClkout, clkout); err != nil {
		return errors.New("pcf8523: disable timer A: " + err.Error())
	}

	// Disable CTAIE and clear CTAF.
	return r.clearTimerFlag()
}

// WakePin returns the GPIO pin connected to the PCF8523 INT1 line.
func (r *RTC) WakePin() uint8 {
	return r.wakePin
}

// clearTimerFlag clears the CTAF flag and disables CTAIE in Control_2.
func (r *RTC) clearTimerFlag() error {
	ctrl2, err := r.read(regControl2)
	if err != nil {
		return errors.New("pcf8523: read control_2: " + err.Error())
	}
	ctrl2 &^= bitCTAF | bitCTAIE
	if err := r.write(regControl2, ctrl2); err != nil {
		return errors.New("pcf8523: clear timer flag: " + err.Error())
	}
	return nil
}

// pickClock selects the finest timer source clock whose count fits in
// a single byte (≤ 255), giving the best resolution for the requested
// duration. Returns an error if the duration exceeds the maximum timer
// range (~4.25 hours).
func pickClock(d time.Duration) (taq, count uint8, err error) {
	ns := int64(d)
	for _, c := range clocks {
		ticks := ns / c.tickNanos
		if ticks >= 1 && ticks <= 255 {
			return c.taq, uint8(ticks), nil
		}
	}
	return 0, 0, errDurationRange
}

// --- I2C helpers for timer registers not covered by the upstream driver ---

func (r *RTC) read(reg uint8) (uint8, error) {
	var buf [1]byte
	err := r.bus.Tx(r.addr, []byte{reg}, buf[:])
	return buf[0], err
}

func (r *RTC) write(reg, val uint8) error {
	return r.bus.Tx(r.addr, []byte{reg, val}, nil)
}
