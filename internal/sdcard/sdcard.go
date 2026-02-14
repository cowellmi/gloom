// Package sdcard provides a high-level SD card interface backed by a
// FAT filesystem. It wraps the TinyGo sdcard and fatfs drivers,
// presenting file-level operations suitable for config loading, data
// logging, and log output.
//
// NOTE: tinyfs/fatfs depends on CGo (ChaN FatFs). If the upstream
// TinyGo issue #3460 is still open, FAT support may not build for
// all targets. Check https://github.com/tinygo-org/tinygo/issues/3460.
package sdcard

import (
	"errors"
	"io"
	"machine"
	"os"

	"tinygo.org/x/drivers/sdcard"
	"tinygo.org/x/tinyfs/fatfs"
)

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

// New initialises the SD card on the given SPI bus and chip-select
// pin, then mounts the FAT filesystem. The SPI bus is configured
// internally by the sdcard driver; the caller should not pre-configure
// it.
func New(spi *machine.SPI, sck, sdo, sdi, cs machine.Pin) (*Card, error) {
	dev := sdcard.New(spi, sck, sdo, sdi, cs)
	if err := dev.Configure(); err != nil {
		return nil, errors.New("sdcard: " + err.Error())
	}

	filesystem := fatfs.New(&dev)
	filesystem.Configure(&fatfs.Config{SectorSize: 512})

	if err := filesystem.Mount(); err != nil {
		return nil, errors.New("sdcard: mount: " + err.Error())
	}

	return &Card{dev: dev, fs: filesystem}, nil
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
		return nil, errors.New("sdcard: file does not support sync")
	}
	return af, nil
}

// Mkdir creates a directory. It is a no-op if the directory already
// exists. Intended for creating log/data subdirectories at startup.
func (c *Card) Mkdir(name string) error {
	return c.fs.Mkdir(name, 0)
}

// Remove deletes a file from the filesystem. Intended for future use
// by retention/pruning logic.
func (c *Card) Remove(name string) error {
	return c.fs.Remove(name)
}
