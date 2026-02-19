// Package debug provides a global debug logger backed by an io.Writer.
// Import it from any package and call debug.Log to emit messages.
// Output is silently dropped when no writer is configured.
//
// Usage in main.go:
//
//	debug.W = myUART   // set once, early
//
// Usage anywhere:
//
//	debug.Log("something happened")
package debug

import "io"

// W is the debug output destination. Set it to a *machine.UART or
// any io.Writer early in main. nil by default (all output dropped).
var W io.Writer

var buf [128]byte

// Log writes msg followed by \r\n to W. No-op when W is nil.
func Log(msg string) {
	if W == nil {
		return
	}
	b := buf[:0]
	b = append(b, msg...)
	b = append(b, '\r', '\n')
	W.Write(b)
}
