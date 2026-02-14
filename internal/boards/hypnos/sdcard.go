package hypnos

import (
	"errors"
	"machine"

	"github.com/cowellmi/gloom/internal/sdcard"
)

const (
	// Chip-select pins for the SD card reader on each Hypnos version.
	csV33 = machine.D11 // Hypnos 3.3
	csV32 = machine.D10 // Hypnos 3.2
)

// probeSDCard tries to initialise the SD card reader using each known
// Hypnos chip-select pin. Returns the Card and detected board version.
// If neither pin yields a working card, returns both probe errors.
func probeSDCard(spi *machine.SPI, sck, sdo, sdi machine.Pin) (*sdcard.Card, string, error) {
	// Try Hypnos 3.3 first (CS = D11).
	card, errV33 := sdcard.New(spi, sck, sdo, sdi, csV33)
	if errV33 == nil {
		return card, "3.3", nil
	}

	// Try Hypnos 3.2 (CS = D10).
	card, errV32 := sdcard.New(spi, sck, sdo, sdi, csV32)
	if errV32 == nil {
		return card, "3.2", nil
	}

	return nil, "", errors.Join(errV33, errV32)
}
