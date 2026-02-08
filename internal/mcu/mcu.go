package mcu

import "machine"

// MCU abstracts the processor-level operations needed for low-power
// sleep. Implementations are chip-specific (e.g. SAMD21, nRF52).
type MCU interface {
	Identifier() string

	// EnableWake arms pin as a wake source for the next deep sleep.
	EnableWake(pin machine.Pin)

	// DisableWake clears pin as a wake source.
	DisableWake(pin machine.Pin)

	// PrepareStandby performs one-time configuration so that external
	// interrupts can wake the processor from deep sleep. It is safe
	// to call multiple times; repeated calls are no-ops.
	PrepareStandby()

	// Standby puts the processor into its deepest sleep mode.
	// Execution halts until a configured wake source fires.
	// USB detach/reattach is handled internally.
	Standby()
}
