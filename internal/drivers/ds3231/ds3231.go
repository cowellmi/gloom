// Package ds3231 implements hal.RTC for the Maxim DS3231 real-time clock.
// It wraps the TinyGo driver and adds probe, wake-alarm, and
// pin-reporting logic that hal.System needs. Lives under drivers/ to
// decouple peripheral implementations from the HAL interfaces they satisfy.
package ds3231

import (
	"errors"
	"time"

	"machine"

	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/wait"

	"tinygo.org/x/drivers"
	driver "tinygo.org/x/drivers/ds3231"
)

// RTC wraps the TinyGo DS3231 driver and satisfies hal.RTC.
type RTC struct {
	dev     *driver.Device
	wakePin uint8
}

// compile-time check
var _ hal.RTC = (*RTC)(nil)

// DS3231 I2C address and register constants for chip identification.
const (
	i2cAddr     = 0x68
	regControl  = 0x0E // DS3231 control register
	bitCONV     = 1 << 5
	convTimeout = 250 * time.Millisecond
)

// Probe detects a DS3231 on the given I2C bus and configures it for
// alarm-based wake. wakePin is the GPIO pin number (as uint8)
// connected to the DS3231 SQW/INT output.
//
// The bus must already be configured. Returns an error if the RTC
// cannot be reached after several retries, or if the device at 0x68
// is not actually a DS3231 (e.g. a PCF8523 which shares the same
// I2C address).
var errNotFound = errors.New("ds3231: not found")

func Probe(bus drivers.I2C, wakePin uint8) (*RTC, error) {
	dev := driver.New(bus)

	if !dev.Configure() {
		return nil, errNotFound
	}

	// The RTC may take a few attempts to respond after power-on.
	// If the SAMD21 I2C SERCOM gets stuck, reconfiguring the bus
	// between retries resets its state machine.
	if err := waitForReady(bus, &dev); err != nil {
		return nil, err
	}

	// The DS3231 and PCF8523 share I2C address 0x68. Distinguish
	// them using a behavioral test: the DS3231 control register
	// (0x0E) has a CONV bit (bit 5) that triggers a temperature
	// conversion and auto-clears when done (~200ms). On the
	// PCF8523, register 0x0E is Timer_CLKOUT_ctrl where bit 5 is
	// TAM (timer A interrupt mode) — a config bit that does NOT
	// auto-clear.
	if err := verify(bus); err != nil {
		return nil, err
	}

	// Clear any pending alarms from a previous session.
	if err := dev.ClearAlarm1(); err != nil {
		return nil, errors.New("ds3231: clear alarm 1: " + err.Error())
	}
	if err := dev.ClearAlarm2(); err != nil {
		return nil, errors.New("ds3231: clear alarm 2: " + err.Error())
	}

	// Disable square-wave output so the INT pin is available for
	// alarm interrupts.
	if err := dev.SetSqwPinMode(driver.SQW_OFF); err != nil {
		return nil, errors.New("ds3231: set sqw pin mode: " + err.Error())
	}

	return &RTC{dev: &dev, wakePin: wakePin}, nil
}

// verify confirms the device at 0x68 is a real DS3231 by setting the
// CONV bit in the control register and waiting for it to auto-clear
// after the temperature conversion completes. A PCF8523 will NOT
// auto-clear the corresponding bit.
func verify(bus drivers.I2C) error {
	var buf [1]byte

	// Read current control register value.
	if err := bus.Tx(i2cAddr, []byte{regControl}, buf[:]); err != nil {
		return errors.New("ds3231: failed to read control register")
	}

	// Set CONV bit to trigger a temperature conversion.
	ctrl := buf[0] | bitCONV
	if err := bus.Tx(i2cAddr, []byte{regControl, ctrl}, nil); err != nil {
		return errors.New("ds3231: failed to write control register")
	}

	// Wait for the conversion to complete (~200ms on a real DS3231).
	wait.For(convTimeout)

	// Read control register again. On a real DS3231, CONV has
	// auto-cleared. On a PCF8523, bit 5 (TAM) stays set.
	if err := bus.Tx(i2cAddr, []byte{regControl}, buf[:]); err != nil {
		return errors.New("ds3231: failed to read control register after CONV")
	}
	if buf[0]&bitCONV != 0 {
		return errors.New("ds3231: CONV bit did not auto-clear (not a DS3231)")
	}

	return nil
}

func (r *RTC) Identifier() string { return "DS3231" }

// ReadTime returns the current time from the DS3231.
func (r *RTC) ReadTime() (time.Time, error) {
	return r.dev.ReadTime()
}

// SetWake programs Alarm 1 to fire at target.
func (r *RTC) SetWake(target time.Time) error {
	return r.dev.SetAlarm1(target, driver.A1_DATE)
}

// ClearWake clears the Alarm 1 flag so the INT pin releases.
func (r *RTC) ClearWake() error {
	return r.dev.ClearAlarm1()
}

// WakePin returns the GPIO pin connected to the DS3231 SQW/INT line.
func (r *RTC) WakePin() uint8 {
	return r.wakePin
}

const (
	// Number of attempts to retry I2C operations during probe.
	probeRetries = 5

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

func waitForReady(bus drivers.I2C, dev *driver.Device) error {
	for attempt := 1; attempt <= probeRetries; attempt++ {
		err := dev.SetRunning(true)
		if err == nil {
			return nil
		}
		// Reconfigure I2C to reset the SERCOM state machine
		// in case the bus is stuck.
		if i2c, ok := bus.(*machine.I2C); ok {
			_ = i2c.Configure(machine.I2CConfig{}) // best-effort
		}
		wait.For(probeRetryDelay)
	}
	return errors.New("ds3231: communication timed out")
}
