// Package samd21 implements hal.MCU for the Microchip SAMD21 Cortex-M0.
// Lives under targets/ to separate target-specific code from the HAL
// interfaces it satisfies.
package samd21

import (
	"device/arm"
	"device/sam"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/wait"
)

// Name is the human-readable identifier for this MCU.
const Name = "ATSAMD21"

// GCLK generator assignments. The SAMD21 has 9 generators (0–8).
// Generator 0 is the 48 MHz main clock configured by TinyGo.
const (
	// gclkWDT is the generator for the watchdog timer clock.
	// Sourced from OSCULP32K with a /32 divider (~1.024 kHz).
	// Does NOT run in standby — the WDT halts during deep sleep.
	gclkWDT = 5

	// gclkEIC is the generator for the EIC peripheral during standby.
	// Sourced from OSCULP32K with run-in-standby enabled so
	// edge-detection works while the CPU is halted.
	gclkEIC = 6
)

// MCU holds SAMD21-specific state for deep-sleep management.
type MCU struct {
	standbyReady bool
	wakeFlags    uint32
	ledPin       machine.Pin
	ledReady     bool
}

// New returns an MCU ready for use.
func New() *MCU {
	return &MCU{}
}

// i2cRecoverClocks is the number of SCL toggles in the bus recovery
// sequence. 9 clocks covers one full byte + ACK, which is the maximum
// a slave can be holding SDA for.
const i2cRecoverClocks = 9

// i2cBitDelay is the half-period delay for bit-banged SCL toggling.
// ~5µs per half-cycle ≈ ~100 kHz, well within I2C standard-mode timing.
const i2cBitDelay = 5 * time.Microsecond

// --- hal.MCU interface ---

func (m *MCU) Identifier() string { return Name }

// ConfigureI2C performs a bit-banged bus recovery sequence, then
// configures the I2C peripheral for normal operation. If a slave
// (e.g. DS3231) was mid-transaction when the MCU reset, it may be
// holding SDA low waiting for clocks. The recovery toggles SCL 9
// times to let the slave finish its byte and release SDA, then
// generates a STOP condition. Equivalent to Linux's i2c_recover_bus().
func (m *MCU) ConfigureI2C(sda, scl uint8) error {
	sdaPin := machine.Pin(sda)
	sclPin := machine.Pin(scl)

	// Drive SCL high, read SDA to check bus state.
	sclPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	sclPin.High()
	wait.For(i2cBitDelay)

	// Only run recovery if SDA is being held low.
	if !sdaPin.Get() {
		// Clock SCL up to 9 times. On each rising edge the slave may
		// release SDA if it has finished its current bit.
		for i := 0; i < i2cRecoverClocks; i++ {
			sclPin.Low()
			wait.For(i2cBitDelay)
			sclPin.High()
			wait.For(i2cBitDelay)

			if sdaPin.Get() {
				break
			}
		}

		// Generate a STOP condition: SDA low → high while SCL is high.
		sdaPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
		sdaPin.Low()
		wait.For(i2cBitDelay)
		sclPin.High()
		wait.For(i2cBitDelay)
		sdaPin.High()
		wait.For(i2cBitDelay)
	}

	// Leave pins as inputs for the I2C peripheral to reconfigure.
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInput})
	sclPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	return machine.I2C0.Configure(machine.I2CConfig{})
}

// ArmWake configures pin as a deep-sleep wake source. It:
//  1. Configures the pin as input with pullup.
//  2. Registers a falling-edge interrupt (the ISR automatically disarms).
//  3. Performs one-time standby clock preparation (reroutes GCLK_EIC
//     to OSCULP32K with run-in-standby, per SAMD21 datasheet §18.6.4).
//  4. Sets the EIC.WAKEUP bit for the pin's external interrupt channel.
//
// The pin number is a uint8 matching machine.Pin's underlying type.
func (m *MCU) ArmWake(pin uint8) error {
	p := machine.Pin(pin)

	p.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	// Register a falling-edge interrupt. The callback is intentionally
	// empty — disarming happens explicitly after Standby returns.
	// If the DS3231 INT pin glitches during the 3.3V→battery
	// transition and fires the ISR before Standby, an empty callback
	// keeps the wake source armed so the real alarm still wakes the CPU.
	if err := p.SetInterrupt(machine.PinFalling, func(_ machine.Pin) {}); err != nil {
		return err
	}

	// One-time standby clock reconfiguration. Must come after the
	// first SetInterrupt which initialises the EIC peripheral.
	m.prepareStandby()

	// Enable the EIC wakeup bit for this channel.
	sam.EIC.WAKEUP.SetBits(1 << extIntChannel(p))

	return nil
}

// DisarmWake tears down pin as a wake source: clears the interrupt
// registration and the EIC wakeup bit. Safe to call even if the pin
// was not previously armed.
func (m *MCU) DisarmWake(pin uint8) {
	p := machine.Pin(pin)
	_ = p.SetInterrupt(0, nil)
	sam.EIC.WAKEUP.ClearBits(1 << extIntChannel(p))
}

// PinFired reports whether pin's EIC channel had its interrupt flag
// set when the MCU last woke from Standby.
func (m *MCU) PinFired(pin uint8) bool {
	ch := extIntChannel(machine.Pin(pin))
	return m.wakeFlags&(1<<ch) != 0
}

// prepareStandby performs one-time clock reconfiguration so that
// external interrupts can wake the processor from standby sleep.
//
// By default, TinyGo routes GCLK_EIC through GCLK0 (48 MHz), which
// is halted during standby. This method reroutes GCLK_EIC to a
// dedicated generator sourced from the ultra-low-power 32 kHz
// oscillator with run-in-standby enabled.
//
// Must be called after the first pin.SetInterrupt call, which
// initialises the EIC peripheral and its default clock routing.
// Repeated calls are no-ops.
func (m *MCU) prepareStandby() {
	if m.standbyReady {
		return
	}

	routeEICClock()
	configureStandbyGenerator()
	disableFlashSleepPowerDown()

	m.standbyReady = true
}

// Standby puts the SAMD21 into standby (deep sleep). The CPU halts
// until an enabled wake source (e.g. EIC external interrupt) fires.
//
// Before sleeping:
//   - USB is detached to avoid host enumeration issues on wake.
//   - SysTick is disabled to prevent a known SAMD21 lock-up where a
//     pending tick fires during the standby entry sequence.
//     See: https://www.avrfreaks.net/forum/samd21-samd21e16b-sporadically-locks-and-does-not-wake-standby-sleep-mode
//
// After wake, SysTick and USB are restored.
func (m *MCU) Standby() {
	m.wakeFlags = 0
	detachUSB()
	arm.SYST.SYST_CSR.ClearBits(arm.SYST_CSR_TICKINT)

	arm.SCB.SCR.SetBits(arm.SCB_SCR_SLEEPDEEP)
	arm.Asm("dsb 0xf") // ensure all memory accesses complete
	arm.Asm("wfi")     // halt until wake interrupt

	// --- execution resumes here after wake ---

	// Snapshot interrupt flags before anything clears them.
	m.wakeFlags = sam.EIC.INTFLAG.Get()

	// Clear SLEEPDEEP so runtime WFI (scheduler idle) enters normal
	// idle sleep, not standby.
	arm.SCB.SCR.ClearBits(arm.SCB_SCR_SLEEPDEEP)

	arm.SYST.SYST_CSR.SetBits(arm.SYST_CSR_TICKINT)
	attachUSB()
}

// EnableWatchdog starts the hardware watchdog with an ~8 second timeout.
// The WDT is clocked from OSCULP32K / 32 (~1.024 kHz) via gclkWDT.
// gclkWDT is configured without RUNSTDBY, but in practice the WDT may
// still fire during standby (possibly due to stale GCLK0 routing from
// TinyGo's runtime init). Callers must use DisableWatchdog/EnableWatchdog
// around standby to prevent resets during intentional sleep.
//
// If PetWatchdog is not called within ~8 seconds during normal
// operation, the MCU resets. This guards against I2C bus lockups
// and other hangs.
func (m *MCU) EnableWatchdog() {
	configureWatchdogClock()

	// 8K cycles at ~1.024 kHz ≈ 8 seconds.
	sam.WDT.CONFIG.Set(sam.WDT_CONFIG_PER_8K)
	syncWDT()

	sam.WDT.CTRL.SetBits(sam.WDT_CTRL_ENABLE)
	syncWDT()
}

// PetWatchdog resets the watchdog countdown, preventing a reset.
func (m *MCU) PetWatchdog() {
	sam.WDT.CLEAR.Set(sam.WDT_CLEAR_CLEAR_KEY)
	syncWDT()
}

// DisableWatchdog stops the hardware watchdog. Used before entering
// standby so a potentially-running WDT clock doesn't reset the MCU
// during intentional deep sleep.
func (m *MCU) DisableWatchdog() {
	sam.WDT.CTRL.ClearBits(sam.WDT_CTRL_ENABLE)
	syncWDT()
}

// --- LED ---

func (m *MCU) ConfigureLED(pin uint8) {
	m.ledPin = machine.Pin(pin)
	m.ledPin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	m.ledReady = true
}

func (m *MCU) LedOn() {
	if m.ledReady {
		m.ledPin.High()
	}
}

func (m *MCU) LedOff() {
	if m.ledReady {
		m.ledPin.Low()
	}
}

// --- clock configuration ---

// routeEICClock switches the EIC peripheral clock from the default
// GCLK0 to gclkEIC. The old routing is disabled first, then
// re-enabled pointing at the new generator.
func routeEICClock() {
	// Disable current EIC clock routing (write ID with CLKEN=0).
	sam.GCLK.CLKCTRL.Set(sam.GCLK_CLKCTRL_ID_EIC << sam.GCLK_CLKCTRL_ID_Pos)
	syncGCLK()

	// Re-enable EIC clock routed through gclkEIC.
	sam.GCLK.CLKCTRL.Set(
		sam.GCLK_CLKCTRL_CLKEN |
			gclkEIC<<sam.GCLK_CLKCTRL_GEN_Pos |
			sam.GCLK_CLKCTRL_ID_EIC<<sam.GCLK_CLKCTRL_ID_Pos,
	)
	syncGCLK()
}

// configureStandbyGenerator sets up gclkEIC as an OSCULP32K generator
// with run-in-standby enabled.
//
// RUNSTDBY is included in the initial Set() rather than applied via a
// separate SetBits() call. GENCTRL is a multiplexed register keyed by
// the ID field — a read-modify-write (SetBits) after the initial Set
// could read back a stale ID due to sync latency, applying RUNSTDBY
// to the wrong generator.
func configureStandbyGenerator() {
	sam.GCLK.GENCTRL.Set(
		sam.GCLK_GENCTRL_GENEN |
			sam.GCLK_GENCTRL_RUNSTDBY |
			sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
			gclkEIC<<sam.GCLK_GENCTRL_ID_Pos,
	)
	syncGCLK()
}

// configureWatchdogClock sets up gclkWDT as an OSCULP32K / 32 generator
// (~1.024 kHz) and routes the WDT peripheral clock to it.
func configureWatchdogClock() {
	// Set divider: 32768 Hz / 32 = 1024 Hz.
	sam.GCLK.GENDIV.Set(
		gclkWDT<<sam.GCLK_GENDIV_ID_Pos |
			32<<sam.GCLK_GENDIV_DIV_Pos,
	)
	syncGCLK()

	// Configure generator: OSCULP32K, no RUNSTDBY.
	sam.GCLK.GENCTRL.Set(
		sam.GCLK_GENCTRL_GENEN |
			sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
			gclkWDT<<sam.GCLK_GENCTRL_ID_Pos,
	)
	syncGCLK()

	// Route WDT peripheral clock to gclkWDT.
	sam.GCLK.CLKCTRL.Set(
		sam.GCLK_CLKCTRL_CLKEN |
			gclkWDT<<sam.GCLK_CLKCTRL_GEN_Pos |
			sam.GCLK_CLKCTRL_ID_WDT<<sam.GCLK_CLKCTRL_ID_Pos,
	)
	syncGCLK()
}

// disableFlashSleepPowerDown applies the SAMD21 errata workaround that
// prevents Flash from fully powering down during sleep, which can cause
// wake-up failures on some chip revisions.
func disableFlashSleepPowerDown() {
	ctrlb := sam.NVMCTRL.CTRLB.Get()
	ctrlb &^= sam.NVMCTRL_CTRLB_SLEEPPRM_Msk
	ctrlb |= sam.NVMCTRL_CTRLB_SLEEPPRM_DISABLED << sam.NVMCTRL_CTRLB_SLEEPPRM_Pos
	sam.NVMCTRL.CTRLB.Set(ctrlb)
}

// --- USB ---

// usbEnabled reports whether the USB peripheral is active.
func usbEnabled() bool {
	return sam.USB_DEVICE.CTRLA.HasBits(sam.USB_DEVICE_CTRLA_ENABLE)
}

// attachUSB re-attaches the USB device to the bus. The host will
// re-enumerate and restore the CDC serial connection.
// No-op if the USB peripheral has not been enabled.
func attachUSB() {
	if !usbEnabled() {
		return
	}
	sam.USB_DEVICE.CTRLB.ClearBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// detachUSB electrically disconnects the USB device from the bus.
// No-op if the USB peripheral has not been enabled.
func detachUSB() {
	if !usbEnabled() {
		return
	}
	sam.USB_DEVICE.CTRLB.SetBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// --- sync helpers ---

// syncGCLK spins until the GCLK peripheral sync completes.
func syncGCLK() {
	for sam.GCLK.STATUS.HasBits(sam.GCLK_STATUS_SYNCBUSY) {
	}
}

// syncWDT spins until the WDT peripheral sync completes.
func syncWDT() {
	for sam.WDT.STATUS.HasBits(sam.WDT_STATUS_SYNCBUSY) {
	}
}

// --- pin mapping ---

// extIntChannel returns the EIC channel (EXTINT number) for a given pin.
// Most SAMD21 pins map as pin % 16, but several have exceptions.
// Mirrors TinyGo's machine_atsamd21.go SetInterrupt mapping.
func extIntChannel(pin machine.Pin) uint8 {
	switch pin {
	case machine.PA24:
		return 12
	case machine.PA25:
		return 13
	case machine.PA27:
		return 15
	case machine.PA28:
		return 8
	case machine.PA30:
		return 10
	case machine.PA31:
		return 11
	default:
		return uint8(pin) % 16
	}
}
