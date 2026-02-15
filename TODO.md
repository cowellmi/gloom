# TODO

Items are ordered by severity: critical (data loss / field failure) first, then high (correctness / memory), medium (conventions / maintainability), low (polish), and finally features.

---

## Critical

### `flush()` in manager discards errors

In `internal/manager/manager.go`, `flush()` calls `Flush()` on the logger and all recorders but never checks the returned errors. For an SD card sink, a failed flush before sleep means data loss. Log the error before entering sleep.

### Serial sink permanently self-disables on a single write error

In `internal/sink/serial/serial.go`, any `Write` error sets `s.w = nil` permanently. For UART this is reasonable, but for USB-CDC the connection is torn down and re-established every sleep/wake cycle. A transient write failure (e.g. host not listening) kills the sink for the entire device lifetime. Consider adding a `Reset(w io.Writer)` method that the manager can call after wake to re-inject the writer, or track an error counter and only disable after N consecutive failures.

---

## High

### `formatTimestamp` in `sink/file` heap-allocates every call

In `internal/sink/file/file.go`, `formatTimestamp` returns `string(buf[:])` which escapes the stack-allocated `[19]byte` to the heap on every call. This runs once per measurement per cycle. On 32KB RAM it adds up. Refactor to accept a `[]byte` parameter and append into the caller's scratch buffer, consistent with how the serial sink formats timestamps.

### Logger silently discards sink errors

In `internal/log/logger.go`, `Log()` calls `WriteLog` but discards the returned error without even an `_ =` assignment. Per project conventions, intentionally discarded errors need `_ =` with a comment. Consider tracking a `failed` bool per target so the logger knows when a sink has died, or at minimum use `_ =` with an explanation. Same issue in `Flush()`.

### `Measure()` allocates `[]Measurement` per call

In `internal/sensor/fake/fake.go` (and any real sensor), `Measure()` returns a freshly allocated `[]sensor.Measurement` slice every call. On 32KB RAM, consider pre-allocating a fixed-size array in the Device struct and returning a slice of it, or changing the interface to `Measure(buf []Measurement) ([]Measurement, error)`.

### `debug.Log` allocates on every call

In `internal/debug/debug.go`, `append([]byte(nil), msg...)` allocates a new `[]byte` on every call. If `debug.Log` is used in any hot path, this adds unnecessary GC pressure on 32KB RAM. Use a package-level `[256]byte` scratch buffer instead, or accept a `[]byte` parameter.

### `WakeExternal` is silently ignored in `step()`

In `internal/manager/manager.go`, the `step()` switch handles `WakeSample` and `WakeHeartbeat` but `WakeExternal` falls through with no log entry. For field debugging, add a `logger.Debug("external wake")` case.

### `Fallback.Sleep` uses `time.Sleep` instead of `wait.For`

In `internal/hal/fallback.go`, `Sleep()` calls `time.Sleep(target.Sub(now))`. The AGENT.md and codebase convention say to avoid `time.Sleep` in favor of `wait.For` because of unreliable TinyGo scheduler behavior after SAMD21 standby wake. Even though Fallback is degraded-mode, it should use `wait.For` for consistency and to avoid issues if Fallback ever runs on bare metal.

---

## Medium

### `maybeRotate` discards `openForDate` error

In `internal/sink/file/file.go`, `maybeRotate` calls `s.openForDate(t)` but ignores the returned error. Per project conventions, this should be `_ = s.openForDate(t)` with a comment, or the error should be propagated so callers know rotation failed.

### `detachUSB` / `BeginSerial` symmetry

In `internal/mcu/samd21/samd21.go`, `detachUSB` guards on USB being enabled but `BeginSerial` does not. They should both check state before acting. Also `detachUSB` is unexported while `BeginSerial`/`EndSerial` are exported -- make the visibility consistent.

### Config boolean values not documented for non-programmers

In `internal/config/config.go`, boolean fields like `serial` and `enable_led` only accept the exact string `"true"` to enable. Any other value (including `"yes"`, `"1"`, `"TRUE"`) silently means false. For a framework targeting non-programmers, either document the accepted values clearly in a sample config file, or accept common truthy variants (`"true"`, `"yes"`, `"1"`, case-insensitive).

### Shared `buf` in Manager needs concurrency note

In `internal/manager/manager.go`, the `[recorderBufSize]byte` scratch buffer is shared across all recorders and `logMem`. Currently single-threaded so it works, but if goroutine-based sensor polling is ever added this becomes a data race. Add a comment documenting the single-goroutine assumption.

### Duplicate `appendLevel` function

Both `internal/sink/serial/serial.go` and `internal/sink/file/file.go` define identical `appendLevel` helper functions. Extract to a shared location (e.g. `internal/log/` or a small `internal/format/` package) to avoid drift.

### Duplicate `earliest()` helper

Both `internal/hal/fallback.go` and `internal/boards/hypnos/hypnos.go` define identical `earliest(a, b time.Time) time.Time` functions. Extract to `internal/hal/` as an exported or unexported helper so the two Platform implementations share one copy.

### Missing test coverage for `internal/log` and `internal/sink/serial`

Both packages show `[no test files]`. The logger fan-out logic (level filtering, multi-sink dispatch) and serial formatting deserve at least basic unit tests, especially since the logger silently swallows errors.

---

## Low

### No CSV header row in data files

Data files are written as bare CSV (`timestamp,device,label,value,unit`) but there's no header row. Adding a header on file creation would make the CSVs self-documenting for researchers working with the data offline.

---

## Features

### File retention / pruning for SD card logs and sensor data

Daily rotation is implemented with files organized into directories: `data/20260214.csv` for sensor recordings and `logs/20260214.log` for log entries. Old files are never deleted. Add configurable retention periods with separate settings for logs and sensor recordings:

- `log_retain_days` — number of days to keep log files (e.g. 7). Diagnostic logs are high-volume and low-value once reviewed; a researcher may want to keep only a few days.
- `data_retain_days` — number of days to keep sensor recordings. Sensor data is the primary scientific output and may need to be kept indefinitely (`0` = keep forever) until manually retrieved from the SD card.

Implementation notes:
- On startup or daily rotation, compute expected old filenames (`{dir}/{YYYYMMDD}{ext}`) going back beyond the retention window and call `card.Remove` for each. This avoids FAT directory listing (which is limited in fatfs) by using predictable date-stamped names.
- `card.Remove` already exists for this purpose.

### Blues Notecard integration (config source + data sink)

Use the Notecard as the primary config source and a data/log sink. The Notecard communicates over I2C (`0x17`) using JSON commands and provides store-and-forward sync to Notehub. SD card becomes a local black-box backup rather than the single source of truth for config.

#### 1. Notecard I2C driver — `internal/notecard/notecard.go`

Low-level I2C JSON request/response wrapper. Build request JSON with `append` (no `encoding/json`). Parse responses with minimal scanning to stay within 32KB RAM. Shared by both the config source and the data sink.

#### 2. Device ID persistence — `internal/mcu/samd21/nvm.go`

Store a short device ID string in a reserved SAMD21 flash row (64 bytes) via the NVM controller. Add `ReadDeviceID() (string, error)` and `WriteDeviceID(id string) error` to the MCU. Flash write cycles (~25K) are fine since the ID rarely changes. Add a `DeviceStore` interface in `internal/mcu/` so this stays target-agnostic.

#### 3. Config from Notecard environment variables

Notehub environment variables are hierarchical (project → fleet → device). Set config keys (`sample_interval`, `sensors`, etc.) from the Notehub dashboard; devices pull them via `env.get`. The Notecard caches env vars locally, so reads succeed even when cellular is down.

#### 4. Boot flow in `main.go`

1. Read device ID from flash. If empty, generate a random one (`"gloom-"` + 4 hex chars) and write it to flash.
2. Add `device_id` to `Config`. Use it as the Notecard's device identity and in CSV/log output.
3. Try `env.get` from the Notecard for config values. On success, cache to SD card `config.ini` as backup.
4. If Notecard unavailable, fall back to SD card `config.ini`.
5. If SD card also unavailable, use `config.Default()`.

This means a freshly flashed device with no SD card still boots, self-identifies, and becomes configurable from the cloud once the Notecard connects.

#### 5. Data sink — `internal/sink/notecard/notecard.go`

Implement `sensor.Recorder` and `log.Sink`. Queue measurements into a `data.qo` Notefile via `note.add`; queue error-level logs into `logs.qo`. Let the Notecard manage its own sync cadence, or optionally `hub.sync` on `Flush()`. From Notehub, data routes to MQTT, HTTP, AWS IoT, etc.

#### 6. SD card role change

SD card shifts from config authority to local backup: cached config, data logging, log files. If the card is missing or corrupt the device still runs. Existing file sink and retention logic stay unchanged.
