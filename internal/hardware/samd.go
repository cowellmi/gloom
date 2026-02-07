// A lot of this was ported via LLM from ArduinoLowPower library:
// https://github.com/arduino-libraries/ArduinoLowPower/blob/master/src/samd/ArduinoLowPower.cpp

package hardware

import (
	"device/arm"
	"device/sam"
	"machine"
)

// EnableWakeOnInterrupt sets the EIC.WAKEUP bit for pin so that its external
// interrupt can wake the SAMD21 from standby sleep.
func EnableWakeOnInterrupt(pin machine.Pin) {
	sam.EIC.WAKEUP.SetBits(1 << pinToEXTINT(pin))
}

// DisableWakeOnInterrupt clears the EIC.WAKEUP bit for pin.
func DisableWakeOnInterrupt(pin machine.Pin) {
	sam.EIC.WAKEUP.ClearBits(1 << pinToEXTINT(pin))
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

// ConfigureEICStandby reconfigures GCLK_EIC to use GCLK6 sourced from the
// ultra-low-power 32 kHz oscillator (OSCULP32K) with run-in-standby enabled.
//
// By default, TinyGo routes GCLK_EIC through GCLK0 (48 MHz), which is halted
// during standby sleep. The SAMD21 datasheet (§18.6.4) states:
//
//	"If edge-detection is required while the CPU is in standby, the
//	 GCLK_EIC must be configured to run in standby mode."
//
// This function must be called after the first pin.SetInterrupt call (which
// initialises the EIC peripheral and its default clock routing).
//
// Ported from the ArduinoLowPower SAMD configGCLK6() helper.
func ConfigureEICStandby() {
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
}

// EnterStandby puts the SAMD21 into standby (deep sleep) mode. The CPU is
// halted until an enabled wake source (e.g. EIC external interrupt) fires.
//
// Before sleeping:
//   - USB is detached from the bus to avoid enumeration issues on wake.
//   - The SysTick interrupt is disabled to prevent a known SAMD21 lock-up
//     where a pending SysTick fires during the standby entry sequence.
//     See: https://www.avrfreaks.net/forum/samd21-samd21e16b-sporadically-locks-and-does-not-wake-standby-sleep-mode
//
// After wake, SysTick and USB are restored.
func EnterStandby() {
	// Detach USB from the bus. On boards where Serial is USB CDC
	// (SERIAL_PORT_USBVIRTUAL), Arduino calls USBDevice.standby() which
	// is a no-op on most SAMD cores. We unconditionally detach to keep
	// the host from seeing a stale device during sleep.
	DetachUSB()

	// Disable SysTick interrupt to prevent standby lock-up.
	arm.SYST.SYST_CSR.ClearBits(arm.SYST_CSR_TICKINT)

	// Select deep sleep (standby) mode.
	arm.SCB.SCR.SetBits(arm.SCB_SCR_SLEEPDEEP)

	// Data Synchronization Barrier — ensure all memory accesses complete
	// before entering standby.
	arm.Asm("dsb 0xf")

	// Wait For Interrupt — halts the CPU until a wake interrupt fires.
	arm.Asm("wfi")

	// --- execution resumes here after wake ---

	// Re-enable SysTick interrupt.
	arm.SYST.SYST_CSR.SetBits(arm.SYST_CSR_TICKINT)

	// Re-attach USB so the host re-enumerates the device.
	BeginSerial()
}

// EndSerial detaches the SAMD21 USB device from the bus, ending the CDC
// serial connection. The host will see the device disconnect and close the
// virtual serial port.
//
// Call machine.Serial.Configure() is not needed to re-establish the
// connection; clearing the DETACH bit via BeginSerial is sufficient since
// the USB descriptors and endpoint configuration remain intact.
func EndSerial() {
	sam.USB_DEVICE.CTRLB.SetBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// BeginSerial re-attaches the SAMD21 USB device to the bus after a
// previous EndSerial call. The host will re-enumerate the device and
// restore the CDC serial connection.
func BeginSerial() {
	sam.USB_DEVICE.CTRLB.ClearBits(sam.USB_DEVICE_CTRLB_DETACH)
}

// DetachUSB electrically disconnects the SAMD21 USB device from the bus.
// Returns false if the USB peripheral has not been enabled (initialized).
//
// Ported from Arduino SAMD core USBCore.cpp:
//
//	bool USBDeviceClass::detach() {
//	    if (!initialized)
//	        return false;
//	    usbd.detach();
//	    return true;
//	}
//
// In Arduino, `initialized` is set at the end of USBDeviceClass::init()
// after the USB peripheral is enabled. The TinyGo equivalent is checking
// whether CTRLA.ENABLE is set. usbd.detach() maps directly to setting
// CTRLB.DETACH, which signals a disconnect to the host.
func DetachUSB() bool {
	if !sam.USB_DEVICE.CTRLA.HasBits(sam.USB_DEVICE_CTRLA_ENABLE) {
		return false
	}
	sam.USB_DEVICE.CTRLB.SetBits(sam.USB_DEVICE_CTRLB_DETACH)
	return true
}
