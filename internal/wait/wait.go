// Package wait provides a scheduler-free delay using a busy-wait
// loop. On bare-metal TinyGo targets this avoids the opaque
// SysTick/scheduler path of time.Sleep, which has shown unreliable
// behavior after SAMD21 standby wake.
//
// There are no other goroutines to yield to on these devices, so
// spinning is functionally equivalent to time.Sleep with zero risk.
package wait

import "time"

// For spins until at least duration d has elapsed.
func For(d time.Duration) {
	start := time.Now()
	for time.Since(start) < d {
		// spin
	}
}
