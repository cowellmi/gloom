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

	// ConfigureI2C performs a bit-banged bus recovery sequence on
	// the given SDA/SCL pins to release a stuck slave, then
	// configures the I2C peripheral for normal operation.
	ConfigureI2C(sda, scl uint8) error

	// ConfigureLED configures the given pin as a push-pull output
	// for use as a status LED. Subsequent LedOn/LedOff calls drive
	// this pin. Safe to call multiple times (reconfigures the pin).
	ConfigureLED(pin uint8)

	// LedOn drives the configured LED pin high (active-high boards)
	// or low (active-low boards). No-op if ConfigureLED was not called.
	LedOn()

	// LedOff drives the configured LED pin low (active-high boards)
	// or high (active-low boards). No-op if ConfigureLED was not called.
	LedOff()

	// Standby puts the MCU into its deepest sleep mode.
	Standby()
}
