package hal

// MCU abstracts the microcontroller operations needed by the System
// for deep sleep and watchdog control. Implementations live in
// targets/<chip>/ (e.g. targets/samd21). Pin numbers use uint8 to
// avoid importing the machine package.
type MCU interface {
	Identifier() string
	EnableWatchdog()
	DisableWatchdog()
	PetWatchdog()

	// ArmWake configures pin as a wake source: sets it as input
	// pullup, registers a falling-edge interrupt (no-op callback),
	// prepares standby clocks, and enables the EIC wakeup bit.
	// The caller must call DisarmWake after Standby returns.
	ArmWake(pin uint8) error

	// DisarmWake tears down the wake source for pin: clears the
	// interrupt registration and the EIC wakeup bit. Safe to call
	// even if the pin was not previously armed.
	DisarmWake(pin uint8)

	// RecoverI2C performs a bit-banged bus recovery sequence on the
	// given SDA/SCL pins. It toggles SCL 9 times to let any slave
	// release SDA, then generates a STOP condition. Must be called
	// before the I2C peripheral is configured.
	RecoverI2C(sda, scl uint8)

	// Standby puts the MCU into its deepest sleep mode.
	Standby()
}
