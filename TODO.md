# TODO

## `detachUSB` / `BeginSerial` symmetry

In `internal/mcu/samd21/samd21.go`, `detachUSB` guards on USB being enabled but `BeginSerial` does not. They should both check state before acting. Also `detachUSB` is unexported while `BeginSerial`/`EndSerial` are exported -- make the visibility consistent.

## GCLK6 RUNSTDBY race in `PrepareStandby`

In `internal/mcu/samd21/samd21.go`, GCLK `GENCTRL` is a multiplexed register keyed by the ID field. The initial `Set()` writes ID=6 correctly, but the subsequent `SetBits(RUNSTDBY)` is a read-modify-write. If the read-back returns a stale ID (due to sync latency or peripheral muxing), the RUNSTDBY bit could land on the wrong generator. Fold RUNSTDBY into the single `Set()` call to eliminate the race and the extra sync loop:

```go
sam.GCLK.GENCTRL.Set(
    sam.GCLK_GENCTRL_GENEN |
        sam.GCLK_GENCTRL_RUNSTDBY |
        sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
        6<<sam.GCLK_GENCTRL_ID_Pos,
)
```

## `formatTimestamp` in `sink/file` heap-allocates every call

In `internal/sink/file/file.go`, `formatTimestamp` returns `string(buf[:])` which escapes the stack-allocated `[19]byte` to the heap on every call. This runs once per measurement per cycle. On 32KB RAM it adds up. Refactor to accept a `[]byte` parameter and append into the caller's scratch buffer, consistent with how the serial sink formats timestamps.

## Logger silently discards sink errors

In `internal/log/logger.go`, `Log()` calls `WriteLog` but discards the returned error without even an `_ =` assignment. Per project conventions, intentionally discarded errors need `_ =` with a comment. Consider tracking a `failed` bool per target so the logger knows when a sink has died, or at minimum use `_ =` with an explanation.

## `flush()` in manager discards errors

In `internal/manager/manager.go`, `flush()` calls `Flush()` on the logger and all recorders but never checks the returned errors. For an SD card sink, a failed flush before sleep means data loss. Log the error before entering sleep.

## Shared `buf` in Manager needs concurrency note

In `internal/manager/manager.go`, the `[recorderBufSize]byte` scratch buffer is shared across all recorders and `logMem`. Currently single-threaded so it works, but if goroutine-based sensor polling is ever added this becomes a data race. Add a comment documenting the single-goroutine assumption.

## `WakeExternal` is silently ignored in `step()`

In `internal/manager/manager.go`, the `step()` switch handles `WakeSample` and `WakeHeartbeat` but `WakeExternal` falls through with no log entry. For field debugging, add a `logger.Debug("external wake")` case.

## `Measure()` allocates `[]Measurement` per call

In `internal/sensor/fake/fake.go` (and any real sensor), `Measure()` returns a freshly allocated `[]sensor.Measurement` slice every call. On 32KB RAM, consider pre-allocating a fixed-size array in the Device struct and returning a slice of it, or changing the interface to `Measure(buf []Measurement) ([]Measurement, error)`.


## Duplicate `appendLevel` function

Both `internal/sink/serial/serial.go` and `internal/sink/file/file.go` define identical `appendLevel` helper functions. Extract to a shared location (e.g. `internal/log/` or a small `internal/format/` package) to avoid drift.

## Blues Notecard sink for cloud connectivity

Add a Notecard sink at `sink/notecard/notecard.go` implementing both `sensor.Recorder` and `log.Sink`. The Notecard is a cellular module that communicates over I2C (address `0x17`) using JSON commands and provides store-and-forward sync to Notehub.

**Measurements:** Use `note.add` to queue readings into a `data.qo` Notefile. Build the JSON payload with `append` (maybe `orsinium-labs/jsony`?) to stay within 32KB RAM. Each Note body carries the device name, label, value, and unit. The Notecard syncs to Notehub on its own schedule -- the MCU never blocks on network.

**Logs:** Queue error-level (or higher) log entries into a `logs.qo` Notefile via `note.add`. Consider using `"sync":true` for critical errors to trigger immediate upload.

**Flush:** Optionally send `hub.sync` on `Flush()` to force a sync before sleep, or let the Notecard manage its own sync cadence to save power.

**Wiring:** Register the Notecard sink in `targets/feather-m0/main.go` alongside the serial and file sinks. I2C is already configured (`machine.I2C0`). The Notecard manages its own modem sleep independently, so the existing sleep/wake cycle does not change.

**Routing:** From Notehub, data can be routed to MQTT brokers, HTTP endpoints, AWS IoT, or any other backend. This removes the need to run an MQTT client on the MCU.
