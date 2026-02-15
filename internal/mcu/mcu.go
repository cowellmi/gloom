package mcu

// MCU abstracts the processor-level operations needed for low-power
// sleep. Implementations are chip-specific (e.g. SAMD21, nRF52).
//
// Pin numbers are passed as uint8 to match machine.Pin's underlying
// type while keeping this package free of machine imports, so code
// that depends on mcu.MCU can be tested with the standard Go
// toolchain.
type MCU interface {
	Identifier() string

	// ArmWake configures pin as a deep-sleep wake source. It sets the
	// pin as an input with pullup, registers a falling-edge interrupt,
	// prepares standby clocks (if not already done), and enables the
	// hardware wake bit. The interrupt handler automatically disarms
	// the wake source when it fires.
	ArmWake(pin uint8) error

	// DisarmWake tears down pin as a wake source, clearing any
	// interrupt registration and wake bits. Safe to call even if
	// the pin was not previously armed.
	DisarmWake(pin uint8)

	// Standby puts the processor into its deepest sleep mode.
	// Execution halts until a configured wake source fires.
	// USB detach/reattach is handled internally.
	Standby()

	// EnableWatchdog starts the hardware watchdog timer. If
	// PetWatchdog is not called within the timeout period, the
	// MCU resets. The watchdog clock should halt during deep sleep
	// so it does not fire while the CPU is intentionally stopped.
	EnableWatchdog()

	// PetWatchdog resets the watchdog countdown. Must be called
	// periodically during normal operation to prevent a reset.
	PetWatchdog()
}
