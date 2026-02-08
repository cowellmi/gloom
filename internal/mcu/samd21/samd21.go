// Package samd21 implements mcu.MCU for the Microchip SAMD21 (Cortex-M0+).
//
// Much of this was ported from the ArduinoLowPower library:
// https://github.com/arduino-libraries/ArduinoLowPower/blob/master/src/samd/ArduinoLowPower.cpp

package samd21

import (
	"device/arm"
	"device/sam"
	"machine"
)

// MCU holds SAMD21-specific state for deep-sleep management.
type MCU struct {
	standbyConfigured bool
}

// New returns an MCU ready for use.
func New() *MCU {
	return &MCU{}
}

// EnableWake sets the EIC.WAKEUP bit for pin so that its external
// interrupt can wake the SAMD21 from standby sleep.
func (m *MCU) EnableWake(pin machine.Pin) {
	sam.EIC.WAKEUP.SetBits(1 << pinToEXTINT(pin))
}

// DisableWake clears the EIC.WAKEUP bit for pin.
func (m *MCU) DisableWake(pin machine.Pin) {
	sam.EIC.WAKEUP.ClearBits(1 << pinToEXTINT(pin))
}

// PrepareStandby reconfigures GCLK_EIC to use GCLK6 sourced from the
// ultra-low-power 32 kHz oscillator (OSCULP32K) with run-in-standby
// enabled. This must be called after the first pin.SetInterrupt call
// (which initialises the EIC peripheral and its default clock routing).
//
// By default, TinyGo routes GCLK_EIC through GCLK0 (48 MHz), which is
// halted during standby sleep. The SAMD21 datasheet (section 18.6.4) states:
//
//	"If edge-detection is required while the CPU is in standby, the
//	 GCLK_EIC must be configured to run in standby mode."
//
// Repeated calls are no-ops.
func (m *MCU) PrepareStandby() {
	if m.standbyConfigured {
		return
	}

	// Disable the current EIC clock routing (write ID with CLKEN=0).
	sam.GCLK.CLKCTRL.Set(sam.GCLK_CLKCTRL_ID_EIC << sam.GCLK_CLKCTRL_ID_Pos)
	for sam.GCLK.STATUS.HasBits(sam.GCLK_STATUS_SYNCBUSY) {
	}

	// Re-enable EIC clock, routed through GCLK6.
	sam.GCLK.CLKCTRL.Set(
		sam.GCLK_CLKCTRL_CLKEN |
			sam.GCLK_CLKCTRL_GEN_GCLK6<<sam.GCLK_CLKCTRL_GEN_Pos |
			sam.GCLK_CLKCTRL_ID_EIC<<sam.GCLK_CLKCTRL_ID_Pos,
	)
	for sam.GCLK.STATUS.HasBits(sam.GCLK_STATUS_SYNCBUSY) {
	}

	// Configure GCLK6 generator: source = OSCULP32K, enable.
	sam.GCLK.GENCTRL.Set(
		sam.GCLK_GENCTRL_GENEN |
			sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
			6<<sam.GCLK_GENCTRL_ID_Pos,
	)
	for sam.GCLK.STATUS.HasBits(sam.GCLK_STATUS_SYNCBUSY) {
	}

	// Enable run-in-standby on GCLK6.
	sam.GCLK.GENCTRL.SetBits(sam.GCLK_GENCTRL_RUNSTDBY)
	for sam.GCLK.STATUS.HasBits(sam.GCLK_STATUS_SYNCBUSY) {
	}

	// Errata: prevent Flash from fully powering down in sleep, which can
	// cause wake-up issues on some SAMD21 revisions.
	ctrlb := sam.NVMCTRL.CTRLB.Get()
	ctrlb &^= sam.NVMCTRL_CTRLB_SLEEPPRM_Msk
	ctrlb |= sam.NVMCTRL_CTRLB_SLEEPPRM_DISABLED << sam.NVMCTRL_CTRLB_SLEEPPRM_Pos
	sam.NVMCTRL.CTRLB.Set(ctrlb)

	m.standbyConfigured = true
}

// Standby puts the SAMD21 into standby (deep sleep) mode. The CPU is
// halted until an enabled wake source (e.g. EIC external interrupt) fires.
//
// Before sleeping:
//   - USB is detached from the bus to avoid enumeration issues on wake.
//   - The SysTick interrupt is disabled to prevent a known SAMD21 lock-up
//     where a pending SysTick fires during the standby entry sequence.
//     See: https://www.avrfreaks.net/forum/samd21-samd21e16b-sporadically-locks-and-does-not-wake-standby-sleep-mode
//
// After wake, SysTick and USB are restored.
func (m *MCU) Standby() {
	detachUSB()

	// Disable SysTick interrupt to prevent standby lock-up.
	arm.SYST.SYST_CSR.ClearBits(arm.SYST_CSR_TICKINT)

	// Select deep sleep (standby) mode.
	arm.SCB.SCR.SetBits(arm.SCB_SCR_SLEEPDEEP)

	// Data Synchronization Barrier — ensure all memory accesses complete.
	arm.Asm("dsb 0xf")

	// Wait For Interrupt — halts CPU until wake interrupt fires.
	arm.Asm("wfi")

	// --- execution resumes here after wake ---

	// Re-enable SysTick interrupt.
	arm.SYST.SYST_CSR.SetBits(arm.SYST_CSR_TICKINT)

	// Re-attach USB so the host re-enumerates the device.
	BeginSerial()
}

// pinToEXTINT returns the EIC channel (EXTINT number) for a given pin.
// Most pins follow pin % 16, but a few SAMD21 pins are exceptions.
// Mirrors the mapping in TinyGo's machine_atsamd21.go SetInterrupt.
func pinToEXTINT(pin machine.Pin) uint8 {
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

// detachUSB electrically disconnects the SAMD21 USB device from the bus.
// No-op if the USB peripheral has not been enabled.
func detachUSB() {
	if !sam.USB_DEVICE.CTRLA.HasBits(sam.USB_DEVICE_CTRLA_ENABLE) {
		return
	}
	sam.USB_DEVICE.CTRLB.SetBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// BeginSerial re-attaches the SAMD21 USB device to the bus after a
// previous detach. The host will re-enumerate the device and restore
// the CDC serial connection.
func BeginSerial() {
	sam.USB_DEVICE.CTRLB.ClearBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// EndSerial detaches the SAMD21 USB device from the bus, ending the
// CDC serial connection.
func EndSerial() {
	sam.USB_DEVICE.CTRLB.SetBits(sam.USB_DEVICE_CTRLB_DETACH)
}
