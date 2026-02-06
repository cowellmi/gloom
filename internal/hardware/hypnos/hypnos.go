package hypnos

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/ds3231"
)

const (
	Rail3V = machine.D5
	Rail5V = machine.D6
)

type Hypnos struct {
	rtc     *ds3231.Device
	version string
}

func (h *Hypnos) Name() string { return "Hypnos " + h.version }

func (h *Hypnos) ReadFile(name string) ([]byte, error) {
	// TODO: read file from SD card
	return nil, errors.New("hypnos: sd card not yet implemented")
}

func (h *Hypnos) ReadTime() (time.Time, error) {
	return h.rtc.ReadTime()
}

func (h *Hypnos) Sleep(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	railsOff()

	reason, err := h.sleepStandby(sampleInterval, heartbeatInterval)
	if err != nil {
		reason = sleepFallback(sampleInterval)
	}

	// Only restore sensor power rails for a sample wake.
	if reason&hardware.WakeSample != 0 {
		railsOn()
		if waitErr := waitForRTC(h.rtc); waitErr != nil {
			err = waitErr
		}
	}

	return reason, err
}

// sleepStandby arms the DS3231 alarms and enters SAMD21
// standby mode. Returns an error if any RTC operation fails.
func (h *Hypnos) sleepStandby(sampleInterval, heartbeatInterval time.Duration) (hardware.WakeReason, error) {
	// TODO:
	// - setup alarms
	// - enter STANDBY sleep mode (at this point execution hangs until interrupt)
	// - detect which interupt triggered wake up (either alarm1, alarm2, or some other interrupt which we can assume is a sensor so we will return hardware.WakeSample). Alarm2 will return hardware.WakeHeartbeat. See WAKEY.md for any clarification.
	//
	// AGENT: checkout ~/projects/gloom/AGENT !!! It has the current implementation that works with out hardware (feather m0 and hypnos board). It also includes the library code (RTClib, Adafruit_SleepyDog (WatchDog) LowPower and the samd data sheet).

	time.Sleep(sampleInterval)

	return hardware.WakeSample, nil
}

// sleepFallback uses time.Sleep when the RTC is unavailable.
// Less power-efficient than standby, but keeps the device
// running. Both sample and heartbeat fire every cycle since
// we can't time them independently without the RTC.
func sleepFallback(sample time.Duration) hardware.WakeReason {
	if sample > 0 {
		time.Sleep(sample)
	}
	return hardware.WakeSample | hardware.WakeHeartbeat
}

const (
	// Number of times to retry I2C operations during probe.
	probeRetries = 3

	// Delay between retries to allow bus recovery.
	probeRetryDelay = 500 * time.Millisecond
)

// Probe I2C for Hypnos components. The I2C bus must already be configured.
func Probe(bus drivers.I2C) (*Hypnos, error) {
	var err error

	defer func() {
		if err != nil {
			railsOff() // Reset machine pins.
		}
	}()

	// Initialize power rails.
	configureRails()
	railsOn()

	rtc := ds3231.New(bus)

	if !rtc.Configure() {
		err = errors.New("hypnos: rtc: internal driver configuration failed")
		return nil, err
	}

	err = waitForRTC(&rtc)
	if err != nil {
		return nil, err
	}

	// Switch the DS3231 SQW/INT pin to alarm interrupt mode.
	err = rtc.SetSqwPinMode(ds3231.SQW_OFF)
	if err != nil {
		return nil, err
	}

	// TODO: detect board revision during probe.
	return &Hypnos{rtc: &rtc, version: "3.3"}, nil
}

func configureRails() {
	Rail3V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	Rail5V.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
}

func railsOn() {
	Rail3V.Low()       // Hypnos 3.3V is Active-Low
	Rail5V.High()      // Hypnos 5V is Active-High
	machine.LED.High() // Visual indicator
}

func railsOff() {
	Rail3V.High()
	Rail5V.Low()
	machine.LED.Low()
}

// I2C can produce transient bus errors right after power is
// restored while pull-ups and the oscillator stabilize. Ping the
// RTC until it responds, same as during initial probe.
func waitForRTC(rtc *ds3231.Device) error {
	for attempt := 0; attempt < probeRetries; attempt++ {
		err := rtc.SetRunning(true)
		if err == nil {
			return nil
		}
		time.Sleep(probeRetryDelay)
	}

	return errors.New("hypnos: rtc communication timed out")
}
