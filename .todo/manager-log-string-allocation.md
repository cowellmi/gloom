# Memory: `string(b)` in manager log calls allocates every cycle

**severity:** low (managed by end-of-cycle GC, but real allocation pressure)

## Location

`internal/manager/manager.go:172,352,388`

```go
m.logger.Debug(string(b))  // appears 3× per sleep/wake cycle
```

## Problem

`b` is a slice into the embedded `m.buf [64]byte` struct field. Converting `[]byte` to `string` always allocates a new heap string in Go — the compiler cannot elide this because `Logger.Debug` takes a `string`. So every cycle produces at least 3 short-lived string allocations (wake log, next-wake log, mem log).

`runtime.GC()` is called at the end of each cycle, so these are reclaimed. But on a SAMD21 with 32KB heap, even transient allocation pressure matters for GC pause time and fragmentation.

## Fix options

**Option A** (minimal): Add a `Logger.DebugBytes(b []byte)` method that passes `[]byte` through to sinks, and update sinks to accept `[]byte`. This is a larger interface change but eliminates the conversion entirely.

**Option B**: Add an unexported `logBytes(level Level, b []byte)` method to `Logger` that internally does `string(b)` only once per sink call and avoids the intermediate allocation. No interface change.

**Option C**: Accept the current behavior. The GC handles it, and on this device the 3 allocations per cycle are tiny compared to the SD write buffers. The `runtime.GC()` call already exists at cycle end specifically for this reason.

Option C is likely the right call for now. Document the intent explicitly with a comment near `runtime.GC()` in `step()`.
