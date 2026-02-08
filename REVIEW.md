# Code Review

## Overall Impression

This is a well-structured TinyGo embedded project. The layered architecture (`hal` -> `boards/hypnos` -> `mcu/samd21`, with `manager` orchestrating) is clean, the allocation discipline is strong (shared scratch buffers, `append`-based string building), and the SAMD21 standby code is well-commented with proper datasheet references. The test coverage on `manager` and `config` is thorough.

That said, there are findings across a few severity tiers.

---

## High Priority

### 1. GCLK6 RUNSTDBY set via read-modify-write on a multiplexed register

In `PrepareStandby` (`internal/mcu/samd21/samd21.go`), GCLK `GENCTRL` is a multiplexed register keyed by the ID field. The initial `Set()` on line 67 writes ID=6 correctly. But line 76 uses `SetBits(RUNSTDBY)`, which is a read-modify-write. If the read-back returns a stale ID (due to sync latency or peripheral muxing), the RUNSTDBY bit could land on the wrong generator. Fold it into the single `Set()` call:

```go
sam.GCLK.GENCTRL.Set(
    sam.GCLK_GENCTRL_GENEN |
        sam.GCLK_GENCTRL_RUNSTDBY |
        sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
        6<<sam.GCLK_GENCTRL_ID_Pos,
)
```

This eliminates the extra sync loop too.

### 2. Heartbeat alarm is never armed on hardware

`sleepStandby` (`internal/boards/hypnos/hypnos.go`) receives `heartbeatInterval` but never uses it. Alarm 2 on the DS3231 is cleared but never set. The entire `doHeartbeat` path in the manager is dead code on actual hardware. If heartbeat is planned, it needs Alarm 2 set and the ISR needs to disambiguate which alarm fired. If it's deferred, mark the `Sleep` signature or add a prominent TODO -- the current `WakeReason` bitmask design implies it's supposed to work.

### 3. `formatTimestamp` in `file/file.go` allocates every call

`string(buf[:])` in `formatTimestamp` (`internal/sink/file/file.go`) escapes to the heap. On a 32KB SAMD21, this runs twice per sensor per cycle (once for data, once for log). Change the signature to append into the caller's scratch buffer:

```go
func appendTimestamp(buf []byte, t time.Time) []byte {
    // append the 19-byte timestamp directly into buf
}
```

This keeps the file sink zero-alloc like the serial sink already is.

---

## Medium Priority

### 4. No config validation for zero/negative intervals

`Parse()` (`internal/config/config.go`) will accept `sample_interval = 0s` or `sample_interval = -1s`. A zero interval with `Fallback.Sleep` creates a tight spin loop. A negative interval sets a DS3231 alarm in the past. Add a validation pass or at minimum clamp to a positive floor.

### 5. `appendLevel` is duplicated

Both `internal/sink/serial/serial.go` and `internal/sink/file/file.go` define their own `appendLevel`. Move it to the `log` package alongside the `Level` type:

```go
// in internal/log/logger.go
func AppendLevel(buf []byte, l Level) []byte { ... }
```

### 6. Sink errors are silently swallowed

In `manager.log()` and `writeMeasurements()` (`internal/manager/manager.go`), the returned errors from sink writes are discarded. The self-disabling pattern in the sinks is good, but the manager has no visibility into whether *all* sinks have died. If serial disconnects and SD fails, the device is operating blind with no way to know. Consider tracking a "healthy sinks" count or at minimum a `println` fallback for catastrophic loss.

### 7. No `Deinit()` / `Close()` on `sensor.Device`

Sensors get `Init()` every cycle (correct since MOSFET rails kill power), but there's no explicit teardown. Some I2C sensors need a clean shutdown sequence before power is cut -- pulling SDA/SCL low while the sensor is mid-transaction can latch the bus. A `Deinit() error` method would give sensors the chance to put themselves in a known state before `powerOff()`.

### 8. No tests for sink formatting

The `serial` and `file` sink packages have no test files. Their formatting logic (timestamps, CSV escaping, level tags) is non-trivial and testable with standard Go -- no `machine` dependency. These deserve table-driven tests.

---

## Low Priority / Nits

### 9. `BeginSerial` / `detachUSB` asymmetry

`detachUSB` (`internal/mcu/samd21/samd21.go`) guards on USB being enabled; `BeginSerial` doesn't. Also `detachUSB` is unexported while `BeginSerial`/`EndSerial` are exported. Inconsistent surface area -- either export both or neither.

### 10. Shared buffer contract is implicit

The `Sink` interface docs say `buf` is scratch space, but nothing prevents a sink from holding a reference to the slice beyond the call. Since all sinks share the same backing array, a retained reference would be silently corrupted on the next call. Add a one-line doc on the interface: "buf must not be retained after the call returns."

### 11. Uncommitted deletion of `internal/hal/base/base.go`

The working tree deletes `base/base.go` and its functionality has been folded into `hal.Fallback` in `hal.go`. This is a clean improvement -- the `base` sub-package was unnecessary indirection. Just make sure to commit it.

### 12. `AGENT.md` content mismatch

`AGENT.md` describes the Hypnos board hardware. If this is context for AI tooling, consider noting that or merging the content into `HYPNOS.md` to avoid confusion.

### 13. Build artifacts not gitignored

`main.bin` and `tio.log` from `targets/hypnos-m0/Makefile` could be committed accidentally. Add them to `.gitignore`.

---

## Summary

The bones are solid. The HAL/board/MCU layering, the allocation-conscious sink design, and the SAMD21 standby implementation show this is written by someone who understands embedded constraints. The highest-impact fixes are the GCLK6 register write (potential hardware bug), the file sink heap allocation (death by a thousand cuts on 32KB), and wiring up the heartbeat alarm or explicitly marking it as unimplemented. The rest is hardening and polish.
