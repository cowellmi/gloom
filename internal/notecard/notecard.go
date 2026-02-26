package notecard

import (
	"errors"
	"strconv"
	"strings"
	"time"

	tinyjson "github.com/andreyvit/tinyjson"

	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/wait"
)

// I2CTxFn is the I2C transaction function used to communicate with the Notecard.
// It matches the signature of hal.I2C.Tx.
type I2CTxFn func(addr uint16, w, r []byte) error

// RawJSON is a pre-encoded JSON value for embedding directly as a field value
// in a RequestResponse call, bypassing recursive encoding.
type RawJSON []byte

// Marshal serializes m to compact JSON bytes. Pre-encode nested structures
// before passing them to RequestResponse to limit marshalMap recursion depth
// and avoid goroutine stack overflow on fixed-stack targets.
func Marshal(m map[string]any) []byte {
	return marshalMap(nil, m)
}

const (
	i2cAddr = 0x17 // Blues Notecard default I2C address
	i2cMax  = 253  // max bytes per I2C chunk (Notecard spec: 255 minus 2-byte header)
	segMax  = 250  // bytes sent before an inter-segment pause
)

// Device is a Blues Notecard attached via I2C.
type Device struct {
	UID string
	tx  I2CTxFn
}

// New probes the Notecard at the default I2C address, reads its device UID,
// and triggers a hub sync. Returns nil and an error if the Notecard is absent
// or unresponsive.
func New(tx I2CTxFn) (*Device, error) {
	d := &Device{tx: tx}

	rsp, err := d.RequestResponse(map[string]any{"req": "card.version"})
	if err != nil {
		return nil, err
	}
	if uid, ok := rsp["device"].(string); ok {
		d.UID = uid
	}

	if err := d.Request(map[string]any{"req": "hub.sync"}); err != nil {
		return nil, err
	}

	return d, nil
}

// Request sends a request to the Notecard and returns an error if the
// Notecard responds with an error field or the I2C transaction fails.
// The response body is discarded.
func (d *Device) Request(req map[string]any) error {
	_, err := d.RequestResponse(req)
	return err
}

// RequestResponse sends a request and returns the parsed response body.
// Returns an error if the Notecard responds with an {"err":"..."} field
// or if the I2C transaction fails.
func (d *Device) RequestResponse(req map[string]any) (map[string]any, error) {
	debug.Log("cp1")
	js := marshalMap(nil, req)
	debug.Log("cp2")
	rspJSON, err := d.transaction(js)
	debug.Log("cp3")
	if err != nil {
		return nil, err
	}
	debug.Log("cp4")
	rsp, err := unmarshalMap(rspJSON)
	debug.Log("cp5")
	if err != nil {
		return nil, err
	}
	debug.Log("cp6")
	if e, ok := rsp["err"].(string); ok && e != "" {
		return nil, errors.New(e)
	}
	debug.Log("cp7")
	return rsp, nil
}

// IsNotFound returns true if err indicates the requested note does not exist.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "note-noexist")
}

// --- I2C transport ---

// i2cWrite sends data to the Notecard using the Blues I2C framing protocol.
// Each write is preceded by a 1ms busy-wait as required by the spec.
func (d *Device) i2cWrite(data []byte) error {
	wait.For(time.Millisecond)
	buf := make([]byte, 1+len(data))
	buf[0] = byte(len(data))
	copy(buf[1:], data)
	return d.tx(i2cAddr, buf, nil)
}

// i2cRead reads up to datalen bytes from the Notecard. The Notecard responds
// with a 2-byte header [available, good] followed by 'good' data bytes.
// Returns the data, the number of bytes still available, and any error.
// Each read is preceded by a 1ms busy-wait as required by the spec.
func (d *Device) i2cRead(datalen int) (data []byte, available int, err error) {
	wait.For(time.Millisecond)
	readbuf := make([]byte, datalen+2)
	var req [2]byte
	req[1] = byte(datalen)
	for i := 0; ; i++ {
		err = d.tx(i2cAddr, req[:], readbuf)
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
func (d *Device) transaction(req []byte) ([]byte, error) {
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
		if err := d.i2cWrite(req[offset : offset+n]); err != nil {
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
		data, available, err := d.i2cRead(chunklen)
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

// --- JSON encoding ---

// marshalMap encodes a map[string]any as compact JSON, appending to buf.
// Supports string, bool, int/int32/int64, uint/uint32/uint64,
// float32/float64, and nested map[string]any values.
func marshalMap(buf []byte, m map[string]any) []byte {
	buf = append(buf, '{')
	first := true
	for k, v := range m {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = strconv.AppendQuote(buf, k)
		buf = append(buf, ':')
		buf = marshalValue(buf, v)
	}
	return append(buf, '}')
}

func marshalValue(buf []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(buf, "null"...)
	case bool:
		if x {
			return append(buf, "true"...)
		}
		return append(buf, "false"...)
	case int:
		return strconv.AppendInt(buf, int64(x), 10)
	case int32:
		return strconv.AppendInt(buf, int64(x), 10)
	case int64:
		return strconv.AppendInt(buf, x, 10)
	case uint:
		return strconv.AppendUint(buf, uint64(x), 10)
	case uint32:
		return strconv.AppendUint(buf, uint64(x), 10)
	case uint64:
		return strconv.AppendUint(buf, x, 10)
	case float32:
		return strconv.AppendFloat(buf, float64(x), 'f', -1, 32)
	case float64:
		return strconv.AppendFloat(buf, x, 'f', -1, 64)
	case string:
		return strconv.AppendQuote(buf, x)
	case RawJSON:
		return append(buf, x...)
	case map[string]any:
		return marshalMap(buf, x)
	default:
		return append(buf, "null"...)
	}
}

// --- JSON decoding ---

// unmarshalMap decodes a JSON object into a map[string]any.
// Nested objects and arrays are stored as raw []byte to avoid the recursive
// tinyjson.Value() call, which overflows the goroutine stack on deeply nested data.
// Panics from the tinyjson parser on malformed input are caught and returned as errors.
func unmarshalMap(data []byte) (result map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch v := r.(type) {
			case error:
				err = errors.New("notecard: json: " + v.Error())
			case string:
				err = errors.New("notecard: json: " + v)
			default:
				err = errors.New("notecard: json: parse error")
			}
		}
	}()
	raw := tinyjson.Raw(data)
	result = make(map[string]any)
	for key := raw.StartObject(); key != nil; key = raw.ContinueObject() {
		k := key.Str()
		switch raw.Peek() {
		case tinyjson.StartObject, tinyjson.StartArray:
			// Extract nested value as raw JSON bytes without recursing.
			// tinyjson.Value() is recursive and overflows on 4+ levels of nesting.
			start := []byte(raw)
			scanNestedValue(&raw)
			result[k] = start[:len(start)-len([]byte(raw))]
		default:
			result[k] = raw.Next().Scalar()
		}
	}
	return result, nil
}

// scanNestedValue advances raw past a JSON object or array using an iterative
// depth counter. raw must be positioned at the opening '{' or '[' (after Peek).
// This is safe on tiny goroutine stacks unlike tinyjson.Skip() which recurses.
func scanNestedValue(raw *tinyjson.Raw) {
	data := []byte(*raw)
	depth := 0
	inStr := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inStr {
			if c == '\\' {
				i++ // skip escaped char
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				*raw = tinyjson.Raw(data[i+1:])
				return
			}
		}
	}
	panic("invalid JSON: unterminated object/array")
}
