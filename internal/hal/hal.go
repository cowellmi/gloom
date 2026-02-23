// Package hal defines the hardware abstraction layer for Gloom.
//
// Pin is the canonical GPIO pin type. All hardware interfaces
// (MCU, RTC, Rails, LED, I2C, SPI) are defined in their own files.
// Concrete implementations live in internal/targets/ and
// internal/drivers/. The sleep/wake platform lives in internal/sleeper/.
package hal

// Pin is a GPIO pin number. It mirrors the underlying type of
// machine.Pin so conversions between the two are trivial.
type Pin uint8

// NoPin is the sentinel value meaning "no pin configured".
// It is intentionally distinct from any valid GPIO pin number.
const NoPin = Pin(0xff)
