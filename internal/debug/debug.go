// Package debug provides a global debug logger that fans out to
// multiple io.Writers. Import it from any package and call debug.Log
// to emit messages. Output is silently dropped when no writers are
// configured.
//
// Usage in main.go:
//
//	debug.Add(myUART)    // set once, early
//	debug.Add(myUSBCDC)
//
// Usage anywhere:
//
//	debug.Log("something happened")
package debug

import "io"

var writers [2]io.Writer
var n int

var buf [128]byte

// Add registers a debug output destination. Callers should add
// writers early in main before any Log calls. Writers beyond the
// internal capacity are silently ignored.
func Add(w io.Writer) {
	if w == nil || n >= len(writers) {
		return
	}
	writers[n] = w
	n++
}

// Reset removes all registered writers. Intended for tests.
func Reset() {
	for i := 0; i < n; i++ {
		writers[i] = nil
	}
	n = 0
}

// Log writes msg followed by \r\n to all registered writers.
// No-op when no writers are configured.
func Log(msg string) {
	if n == 0 {
		return
	}
	b := buf[:0]
	b = append(b, msg...)
	b = append(b, '\r', '\n')
	for i := 0; i < n; i++ {
		writers[i].Write(b)
	}
}
