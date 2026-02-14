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

// Card holds a mounted FAT filesystem on an SD card and tracks files
// opened via OpenAppend so they can be synced in bulk before sleep.
type Card struct {
	dev   sdcard.Device
	fs    *fatfs.FATFS
	files []syncer
}

// syncer is satisfied by *fatfs.File which has a Sync method.
type syncer interface {
	io.Writer
	Sync() error
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
	return f.Close()
}

// OpenAppend opens (or creates) a file for append writing. The file is
// tracked internally so that Sync flushes it. Callers should not close
// the returned writer; it remains open for the device lifetime.
func (c *Card) OpenAppend(name string) (io.Writer, error) {
	f, err := c.fs.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
	if err != nil {
		return nil, err
	}

	// The concrete *fatfs.File has a Sync method. Capture it so
	// Card.Sync can flush all open files before sleep.
	if sf, ok := f.(syncer); ok {
		c.files = append(c.files, sf)
		return sf, nil
	}

	// Fallback: file does not support Sync. Still usable for writes.
	return f, nil
}

// Sync flushes all tracked files to the physical SD card. Intended as
// the sync callback passed to file.Sink so data is durable before the
// MCU enters sleep.
func (c *Card) Sync() error {
	var errs []error
	for _, f := range c.files {
		if err := f.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
