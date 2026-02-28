package notecard

import (
	"errors"
	"time"

	"github.com/cowellmi/gloom/internal/wait"
)

const (
	i2cAddr = 0x17 // Blues Notecard default I2C address
	i2cMax  = 253  // max bytes per I2C chunk (Notecard spec: 255 minus 2-byte header)
	segMax  = 250  // bytes sent before an inter-segment pause
)

var (
	I2CReadFailed = errors.New("notecard: i2c read failed")
	I2COverflow   = errors.New("notecard: i2c: available overflow")
	I2CMismatch   = errors.New("notecard: i2c: count mismatch")
)

type i2cReadError struct{ cause error }

func (e *i2cReadError) Error() string   { return "notecard: i2c read: " + e.cause.Error() }
func (e *i2cReadError) Unwrap() error   { return e.cause }
func (e *i2cReadError) Is(t error) bool { return t == I2CReadFailed }

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
	var res []byte
	chunklen := 0
	pollIter := 0
	for {
		data, available, err := c.i2cRead(chunklen)
		if err != nil {
			return nil, err
		}
		res = append(res, data...)

		// Done when we have a newline and no more bytes are pending.
		if len(res) > 0 && res[len(res)-1] == '\n' && available == 0 {
			return res, nil
		}

		chunklen = available
		if chunklen > i2cMax {
			chunklen = i2cMax
		}
		if chunklen == 0 {
			if len(res) == 0 {
				pollIter++
				if pollIter > maxPollIter {
					return nil, ResponseTimeout
				}
			}
			wait.For(10 * time.Millisecond)
		}
	}
}

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
			return nil, 0, &i2cReadError{err}
		}
		wait.For(2 * time.Millisecond)
	}
	available = int(readbuf[0])
	if available > 253 {
		return nil, 0, I2COverflow
	}
	good := int(readbuf[1])
	if good > datalen {
		return nil, 0, I2CMismatch
	}
	return readbuf[2 : 2+good], available, nil
}
