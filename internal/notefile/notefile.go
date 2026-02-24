// Package notefile exposes Blues Notefiles as io.ReadWriteCloser values.
//
// Kind is inferred from the filename extension:
//
//	.db  — InboundOutbound: Read + Write
//	.qo  — OutboundOnly:   Write only
//	.qi  — InboundOnly:    Read only
//
// Each Write call queues one Note (note.add). Each Read call fetches
// and deletes the next available Note (note.get with delete:true).
package notefile

import (
	"errors"
	"io"
	"strings"

	"github.com/cowellmi/gloom/internal/notecard"
)

// ErrReadOnly is returned by Write when the Notefile is inbound-only (.qi).
var ErrReadOnly = errors.New("notefile: write not allowed on read-only file")

// ErrWriteOnly is returned by Read when the Notefile is outbound-only (.qo).
var ErrWriteOnly = errors.New("notefile: read not allowed on write-only file")

type kind uint8

const (
	kindInboundOutbound kind = iota // .db
	kindWriteOnly                   // .qo
	kindReadOnly                    // .qi
)

// Notefile wraps a Blues Notefile via a Requester transport.
type Notefile struct {
	nc   notecard.Requester
	name string
	kind kind
}

// New creates a Notefile. The extension of name (.db, .qo, .qi)
// determines which operations are permitted.
func New(nc notecard.Requester, name string) (*Notefile, error) {
	var k kind
	switch {
	case strings.HasSuffix(name, ".db"):
		k = kindInboundOutbound
	case strings.HasSuffix(name, ".qo"):
		k = kindWriteOnly
	case strings.HasSuffix(name, ".qi"):
		k = kindReadOnly
	default:
		return nil, errors.New("notefile: unknown extension for " + name)
	}
	return &Notefile{nc: nc, name: name, kind: k}, nil
}

// Write converts p to a Note and queues it via note.add.
// Body: {"data": string(p)}. Each call produces one Note.
// Returns ErrReadOnly if the Notefile is .qi.
func (f *Notefile) Write(p []byte) (int, error) {
	if f.kind == kindReadOnly {
		return 0, ErrReadOnly
	}
	req := map[string]any{
		"req":  "note.add",
		"file": f.name,
		"body": map[string]any{"data": string(p)},
	}
	if err := f.nc.Request(req); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Read fetches the next Note via note.get (delete:true) and copies
// body["data"] into p. Returns io.EOF when no Note is available.
// Returns ErrWriteOnly if the Notefile is .qo.
func (f *Notefile) Read(p []byte) (int, error) {
	if f.kind == kindWriteOnly {
		return 0, ErrWriteOnly
	}
	req := map[string]any{
		"req":    "note.get",
		"file":   f.name,
		"delete": true,
	}
	rsp, err := f.nc.RequestResponse(req)
	if err != nil {
		return 0, err
	}
	body, _ := rsp["body"].(map[string]any)
	if body == nil {
		return 0, io.EOF
	}
	data, _ := body["data"].(string)
	n := copy(p, data)
	return n, nil
}

// Close is a no-op. The Notecard syncs Notefiles independently.
func (f *Notefile) Close() error { return nil }
