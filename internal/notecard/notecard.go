package notecard

import (
	"errors"
	"strings"
	"time"

	"github.com/buger/jsonparser"

	"github.com/cowellmi/gloom/internal/wait"
)

// I2CTxFn is the I2C transaction function used to communicate with the Notecard.
// It matches the signature of hal.I2C.Tx.
type I2CTxFn func(addr uint16, w, r []byte) error

const (
	i2cAddr = 0x17 // Blues Notecard default I2C address
	i2cMax  = 253  // max bytes per I2C chunk (Notecard spec: 255 minus 2-byte header)
	segMax  = 250  // bytes sent before an inter-segment pause
)

// Client is a Blues Notecard attached via I2C.
type Client struct {
	UID string
	tx  I2CTxFn
}

// New probes the Notecard at the default I2C address, reads its device UID,
// and triggers a hub sync. Returns nil and an error if the Notecard is absent
// or unresponsive.
func New(tx I2CTxFn) (*Client, error) {
	c := &Client{tx: tx}

	rsp, err := c.Do([]byte(`{"req":"card.version"}`))
	if err != nil {
		return nil, err
	}
	if uid, err := jsonparser.GetString(rsp, "device"); err == nil {
		c.UID = uid
	}

	if _, err := c.Do([]byte(`{"req":"hub.sync"}`)); err != nil {
		return nil, err
	}

	return c, nil
}

// Do sends a raw JSON request to the Notecard and returns the raw JSON response.
// Returns an error if the Notecard responds with an {"err":"..."} field or if
// the I2C transaction fails.
func (c *Client) Do(req []byte) ([]byte, error) {
	rsp, err := c.transaction(req)
	if err != nil {
		return nil, err
	}
	if e, err := jsonparser.GetString(rsp, "err"); err == nil && e != "" {
		return nil, errors.New(e)
	}
	return rsp, nil
}

// IsNotFound returns true if err indicates the requested note does not exist.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "note-noexist")
}

// --- I2C transport ---

// i2cWrite sends data to the Notecard using the Blues I2C framing protocol.
// Each write is preceded by a 1ms busy-wait as required by the spec.
func (c *Client) i2cWrite(data []byte) error {
	wait.For(time.Millisecond)
	buf := make([]byte, 1+len(data))
	buf[0] = byte(len(data))
	copy(buf[1:], data)
	return c.tx(i2cAddr, buf, nil)
}

// i2cRead reads up to datalen bytes from the Notecard. The Notecard responds
// with a 2-byte header [available, good] followed by 'good' data bytes.
// Returns the data, the number of bytes still available, and any error.
// Each read is preceded by a 1ms busy-wait as required by the spec.
func (c *Client) i2cRead(datalen int) (data []byte, available int, err error) {
	wait.For(time.Millisecond)
	readbuf := make([]byte, datalen+2)
	var req [2]byte
	req[1] = byte(datalen)
	for i := 0; ; i++ {
		err = c.tx(i2cAddr, req[:], readbuf)
		if err == nil {
			break
		}
		if i >= 10 {
			return nil, 0, errors.New("notecard: i2c read: " + err.Error())
		}
		wait.For(2 * time.Millisecond)
	}
	available = int(readbuf[0])
	if available > 253 {
		return nil, 0, errors.New("notecard: i2c read: available overflow")
	}
	good := int(readbuf[1])
	if good > datalen {
		return nil, 0, errors.New("notecard: i2c read: count mismatch")
	}
	return readbuf[2 : 2+good], available, nil
}

// transaction sends the JSON request bytes to the Notecard (appending a
// newline terminator) and reads back the JSON response. The Notecard protocol
// sends requests and responses as newline-terminated JSON over I2C chunks.
func (c *Client) transaction(req []byte) ([]byte, error) {
	// Append newline terminator required by the Notecard protocol.
	req = append(req, '\n')

	// Write the request in chunks no larger than i2cMax bytes.
	// A 250ms inter-chunk delay prevents overflowing the Notecard's interrupt buffer.
	sentInSeg := 0
	for offset := 0; offset < len(req); {
		n := len(req) - offset
		if n > i2cMax {
			n = i2cMax
		}
		if err := c.i2cWrite(req[offset : offset+n]); err != nil {
			return nil, err
		}
		offset += n
		sentInSeg += n
		if sentInSeg >= segMax {
			sentInSeg = 0
			wait.For(250 * time.Millisecond)
		}
		wait.For(250 * time.Millisecond)
	}

	// Poll for the response. Timeout applies only before the first byte is
	// received; once data starts arriving, we read until the newline terminator.
	const maxPollIter = 6000 // 6000 × 10ms = 60s before first byte
	var rsp []byte
	chunklen := 0
	pollIter := 0
	for {
		data, available, err := c.i2cRead(chunklen)
		if err != nil {
			return nil, err
		}
		rsp = append(rsp, data...)

		// Done when we have a newline and no more bytes are pending.
		if len(rsp) > 0 && rsp[len(rsp)-1] == '\n' && available == 0 {
			return rsp, nil
		}

		chunklen = available
		if chunklen > i2cMax {
			chunklen = i2cMax
		}
		if chunklen == 0 {
			if len(rsp) == 0 {
				pollIter++
				if pollIter > maxPollIter {
					return nil, errors.New("notecard: response timeout")
				}
			}
			wait.For(10 * time.Millisecond)
		}
	}
}
