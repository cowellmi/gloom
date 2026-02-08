package hypnos

import (
	"errors"
	"io"
)

// SDCard provides file-like access to the Hypnos SD card reader.
// The caller opens writers for data and logs, and calls Sync to
// flush buffered writes to the FAT before power-down.
type SDCard struct {
	// TODO: FAT filesystem handle, SPI bus reference
}

// OpenWriter opens a file on the SD card for append-only writing.
// TODO: implement using TinyFS or a minimal FAT driver.
func (sd *SDCard) OpenWriter(name string) (io.Writer, error) {
	_ = name
	return nil, errors.New("hypnos: sd not yet implemented")
}

// Sync flushes all buffered writes to the FAT.
// Must be called before powering down the SD rail.
// TODO: implement FAT sync.
func (sd *SDCard) Sync() error {
	return nil // TODO
}
