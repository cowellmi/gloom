//go:build tinygo

// Package sdcard provides a high-level SD card interface backed by a
// FAT filesystem. It wraps the TinyGo sdcard and fatfs drivers,
// presenting file-level operations suitable for config loading, data
// logging, and log output.
package sdcard

import (
	"errors"
	"io"
	"machine"
	"os"
	"strconv"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/sdcard"
	"tinygo.org/x/tinyfs/fatfs"

	"github.com/cowellmi/gloom/internal/hal"
)

// mountRetries is the number of mount attempts before falling back to
// a reformat. A transient SPI glitch can cause a single mount to fail
// even when the filesystem is intact; retrying avoids data loss.
const mountRetries = 3

// File combines write, sync, and close operations for a file opened
// in append mode on the SD card. The concrete *fatfs.File satisfies
// this interface.
type File interface {
	io.WriteCloser
	Sync() error
}

// Card holds a mounted FAT filesystem on an SD card.
type Card struct {
	dev sdcard.Device
	fs  *fatfs.FATFS
}

// NewCard initialises the SD card on the given SPI bus and chip-select
// pin, then mounts the FAT filesystem. The SPI bus is configured
// internally by the sdcard driver; the caller should not pre-configure
// it. spi must be a *machine.SPI; the drivers.SPI interface is used
// so callers don't need to import the machine package.
func NewCard(spi drivers.SPI, sck, sdo, sdi, cs hal.Pin) (*Card, error) {
	dev := sdcard.New(
		spi.(*machine.SPI),
		machine.Pin(sck), machine.Pin(sdo), machine.Pin(sdi), machine.Pin(cs),
	)
	if err := dev.Configure(); err != nil {
		return nil, err
	}

	filesystem := fatfs.New(&dev)
	filesystem.Configure(&fatfs.Config{SectorSize: 512})

	// Attempt mount up to mountRetries times before resorting to a
	// reformat. The first mount can fail due to a transient SPI
	// glitch even though the filesystem is intact -- a retry avoids
	// destroying data unnecessarily.
	var mountErr error
	for attempt := 0; attempt < mountRetries; attempt++ {
		mountErr = filesystem.Mount()
		if mountErr == nil {
			return &Card{dev: dev, fs: filesystem}, nil
		}
	}

	// All mount attempts exhausted. Return the last error so the
	// caller can treat it as fatal (blink LED, halt).
	return nil, errors.New("mount failed after " + strconv.Itoa(mountRetries) + " attempts: " + mountErr.Error())
}

// ReadFile reads an entire file into a byte slice. Intended for small
// files such as configuration. Returns an error if the file does not
// exist or cannot be read.
func (c *Card) ReadFile(name string) ([]byte, error) {
	f, err := c.fs.OpenFile(name, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	buf := make([]byte, size)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// WriteFile creates (or overwrites) a file with the given contents.
// Intended for one-shot writes like seeding a default config file.
func (c *Card) WriteFile(name string, data []byte) error {
	f, err := c.fs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		f.Close()
		return err
	}
	// Sync FAT metadata to disk before close so the directory entry
	// is durable even if the SD card loses power shortly after.
	if sf, ok := f.(File); ok {
		if err := sf.Sync(); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// OpenAppend opens (or creates) a file for append writing. The caller
// is responsible for syncing and closing the returned File.
func (c *Card) OpenAppend(name string) (File, error) {
	f, err := c.fs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
	if err != nil {
		return nil, err
	}

	af, ok := f.(File)
	if !ok {
		// The concrete *fatfs.File always satisfies File. If this
		// fails something is very wrong with the driver.
		f.Close()
		return nil, errors.New("file does not support sync")
	}
	return af, nil
}

// Mkdir creates a directory. It is a no-op if the directory already
// exists. Intended for creating log/data subdirectories at startup.
func (c *Card) Mkdir(name string) error {
	err := c.fs.Mkdir(name, 0)
	if err == nil {
		return nil
	}
	// FatFs returns FR_EXIST (8) when the directory already exists.
	// Treat this as success — the caller just wants the dir to be
	// present, not necessarily freshly created.
	if isExistError(err) {
		return nil
	}
	return err
}

// isExistError returns true if the error indicates the path already
// exists. ChaN FatFs surfaces this as error code 8 (FR_EXIST).
func isExistError(err error) bool {
	// The tinyfs fatfs wrapper formats errors as "fatfs: (N) ...",
	// where N is the FRESULT code. FR_EXIST = 8.
	msg := err.Error()
	for i := 0; i < len(msg)-2; i++ {
		if msg[i] == '(' && msg[i+1] == '8' && msg[i+2] == ')' {
			return true
		}
	}
	return false
}
