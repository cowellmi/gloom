package hal

// MCU abstracts the microcontroller operations needed by the System
// for deep sleep and watchdog control. Implementations live in
// mcu/<chip>/ (e.g. mcu/samd21). Pin numbers use uint8 to
// avoid importing the machine package.
type MCU interface {
	// Identifier returns a human-readable name for the MCU
	// (e.g. "ATSAMD21"), used in boot banners and diagnostics.
	Identifier() string

	// EnableWatchdog starts the hardware watchdog timer. Once
	// enabled, PetWatchdog must be called periodically or the
	// MCU will reset. Must be disabled before Standby to avoid
	// resets during intentional deep sleep.
	EnableWatchdog()

	// DisableWatchdog stops the hardware watchdog timer. Called
	// before Standby so the watchdog doesn't fire while the CPU
	// is halted.
	DisableWatchdog()

	// PetWatchdog resets the watchdog countdown, preventing a
	// reset. Must be called at regular intervals while the
	// watchdog is enabled.
	PetWatchdog()

	// ArmWake configures pin as a wake source: sets it as input
	// pullup, registers a falling-edge interrupt (no-op callback),
	// prepares standby clocks, and enables the EIC wakeup bit.
	// The caller must call DisarmWake after Standby returns.
	ArmWake(pin Pin) error

	// DisarmWake tears down the wake source for pin: clears the
	// interrupt registration and the EIC wakeup bit. Safe to call
	// even if the pin was not previously armed.
	DisarmWake(pin Pin)

	// PinFired reports whether pin's interrupt flag was set when
	// the MCU last woke from Standby. The flag is captured before
	// DisarmWake clears it, so callers may check at any point
	// after Standby returns.
	PinFired(pin Pin) bool

	// PinActive reports whether pin is currently in its asserted state.
	// For all wake sources in this system, asserted means the pin is
	// driven low (active-low: RTC INT, sensor DRDY, etc.).
	// Can be polled without entering Standby. Only meaningful while
	// ArmWake has been called for the pin (which configures it as
	// PinInputPullup so the idle state is high).
	PinActive(pin Pin) bool

	// ConfigureI2C performs a bit-banged bus recovery sequence on
	// the given SDA/SCL pins to release a stuck slave, then
	// configures the I2C peripheral for normal operation.
	ConfigureI2C(sda, scl Pin) error

	// Standby puts the MCU into its deepest sleep mode.
	Standby()

	// PaintStack fills the unused portion of the stack region with a
	// sentinel pattern (0xDEADBEEF). Must be called as early as
	// possible in main — before deep call chains execute — so the
	// watermark captures the true high-water mark.
	PaintStack()

	// StackSize returns the total stack region size in bytes as set
	// by the linker (e.g. tinygo -stack-size=4KB). Returns 0 if the
	// target does not expose the linker symbol.
	StackSize() uint

	// StackUsed returns the number of stack bytes that have been touched
	// since PaintStack was called (size minus remaining sentinel words).
	// Returns 0 if PaintStack was never called.
	StackUsed() uint
}
