package hal

// MCU abstracts the microcontroller operations needed by the System
// for deep sleep and watchdog control. Implementations live in
// mcu/<chip>/ (e.g. mcu/samd21). Pin numbers use uint8 to avoid
// importing the machine package.
type MCU interface {
	Identifier() string
	EnableWatchdog()
	DisableWatchdog()
	PetWatchdog()

	// ArmWake configures pin as a wake source: sets it as input
	// pullup, registers a falling-edge interrupt, prepares standby
	// clocks, and enables the EIC wakeup bit. The interrupt handler
	// is internal and disarms the wake source on fire.
	ArmWake(pin uint8) error

	// DisarmWake tears down the wake source for pin.
	DisarmWake(pin uint8)

	// Standby puts the MCU into its deepest sleep mode.
	Standby()
}
