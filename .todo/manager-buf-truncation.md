# Memory: `Manager.buf` is 64 bytes — wake log can silently truncate

**severity:** low

## Location

`internal/manager/manager.go:58`

```go
buf [64]byte
```

Used by `step()` to build the `"wake: group1 group2 ..."` log line and by
`logNextWake` and `logMem`.

## Problem

`fmtbuf` silently truncates at `cap(b)`. The wake log prefix is `"wake: "` (6 bytes), leaving 58 bytes for group names and spaces. The mem log is `"mem: heap_alloc=123456B stack_alloc=9876B"` (~42 bytes), fine. The next-wake log is `"sleep: next wake in 7d"` (~22 bytes), fine.

For the wake log: with 4 groups named `"weather"`, `"heartbeat"`, `"motion"`, `"backup"` all firing simultaneously, the output is `"wake: weather heartbeat motion backup"` = 37 bytes — fine. But longer or more group names (e.g. `"rain_gauge_primary"`, `"air_quality_node_1"`) approach the limit. Truncation produces a misleading partial message with no indication that it was cut.

## Fix

Double the buffer to 128 bytes. It's embedded in the heap-allocated `Manager` struct, so it costs 64 additional bytes of heap — negligible. The `logNextWake` and `logMem` methods share the same buffer sequentially (not simultaneously), so there is no aliasing concern.

```go
buf [128]byte
```

No other changes needed. `fmtbuf` respects `cap(b)` automatically.
