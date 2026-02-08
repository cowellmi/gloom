## Agent Prompt: Config Error Collection, Base Platform Comment, Logger Error Handling, and Output Architecture Planning

### Context

You are working on **Gloom**, a TinyGo-based embedded data logger targeting the Feather M0 + Hypnos FeatherWing. The Hypnos board has a DS3231 RTC, SD card reader, and two MOSFET-controlled power rails. The codebase lives under `/workspace`. Read `AGENT.md` for hardware context.

Key files you will be modifying or studying:

- `internal/config/config.go` and `internal/config/config_test.go`
- `internal/platform/base/base.go`
- `internal/log/logger.go` and `internal/log/logger_test.go`
- `internal/manager/manager.go` and `internal/manager/manager_test.go`
- `internal/sensors/sensor.go`
- `internal/platform/platform.go`
- `targets/hypnos-m0/main.go`

Run tests with `make test` from the project root.

---

### Task 1: Collect errors in `config.Parse` instead of returning on first error

Currently `config.Parse` returns immediately on the first error (e.g. an invalid duration). Change it to **collect all parse errors** and return them together so that a caller (like `main.go`) can log every problem at once.

Requirements:

- `config.Parse` should continue parsing after encountering an error on a given line.
- Accumulate errors and return them joined via `errors.Join` (or a slice -- your call on the API, but `errors.Join` is clean).
- Unknown keys should produce an error (e.g. `"unknown config key: sampl_interval"`). This catches typos on a headless device where debugging is painful.
- An unrecognized `log_level` value should produce an error rather than silently keeping the default.
- The `sensors` key should **reset** `cfg.Sensors` to empty before appending, so that calling `Parse` twice doesn't accumulate duplicate sensor IDs.
- Add config keys for `enable_led` (bool, like `serial`/`wait_for_serial`) and `max_wait_for_serial` (duration, like `sample_interval`). These fields exist on `Config` but are not parseable from file today.
- Update `config_test.go`:
  - Test that multiple errors are collected (e.g. two bad durations in one file).
  - Test that unknown keys produce an error.
  - Test that invalid `log_level` produces an error.
  - Test that `sensors` key replaces rather than appends on re-parse.
  - Test the new `enable_led` and `max_wait_for_serial` keys.
- Update `targets/hypnos-m0/main.go` to handle the new error return from `Parse`. Currently it does `err = config.Parse(data, &cfg)` and appends a single error. Since `Parse` now returns a joined error, just keep appending that single (joined) error to `initErrs` -- `errors.Join` already produces a multi-line string via `.Error()`. Or if you change the return to `[]error`, append them all. Either way, every parse problem must surface in the init-error log dump.

---

### Task 2: Comment on `base.System.Sleep` ignoring `heartbeatInterval`

In `internal/platform/base/base.go`, the `Sleep` method ignores its second parameter. Add a comment explaining the reasoning:

> Base is the degraded-mode fallback used when the primary platform (e.g. Hypnos) fails to probe. It only supports sampling on a simple `time.Sleep` interval. Heartbeat is intentionally unsupported here: heartbeat intervals are only configured on platforms that can actually deliver them (e.g. Hypnos with its RTC alarms). If a researcher has configured a heartbeat interval and the device falls back to Base, the absence of heartbeat messages in the upstream system is itself the signal that something is wrong with that device -- prompting the researcher to go physically check the device and its logs.

Adapt the wording to be concise but capture this intent. Place it as a doc comment on the `Sleep` method.

---

### Task 3: Handle `Logger.Write` errors

In `internal/log/logger.go`, `l.w.Write(buf)` discards the error. Change this:

- If `Write` returns an error, the logger should **disable itself** by setting `l.w = nil`. This way subsequent log calls become no-ops rather than repeatedly failing on a dead writer.
- Do NOT attempt to log the write error (that would recurse). The silent self-disable is the correct embedded behavior -- if serial dies or the SD card is pulled, stop trying.
- Add a brief comment explaining this design choice.
- Update `logger_test.go` with a test using a writer that returns an error, verifying that subsequent writes are suppressed (buffer should not grow after the failing write).

---

### Task 4: Design the output/sink architecture

This is the planning task. The goal is to design how Gloom will output data (logs and sensor measurements) to multiple destinations. **Do not implement SD card I/O or networking.** Produce the architecture as **code: new/modified interfaces, types, and skeleton files** with doc comments and TODOs. The implementation will come later.

#### Current state

- `log.Logger` writes formatted text lines to a single `io.Writer` (serial).
- Sensor measurements are formatted as log lines by `manager.doSample()` and written through the same logger.
- There is no structured data output, no SD card writing, no network publishing.

#### Requirements and constraints

1. **Local-first**: SD card is the immediate next implementation target. The Hypnos board has an SD card reader. We want to write both logs and sensor measurements to files on the SD card.

2. **Sensor data is not just logs**: Measurements need to be written in a structured format (e.g. CSV or a simple binary record) suitable for later retrieval and analysis. They should not only flow through the text logger. The logger is for operational/debug messages; sensor data is the primary payload.

3. **Future network publishing**: After SD card, we will add the ability to publish data over the air. Planned transports include:
   - **Blues Notecard** (LTE/WiFi) -- communicates over I2C/UART, publishes JSON to Notehub (cloud).
   - **LoRa** -- short packet radio to an adjacent gateway node.
   - **MQTT** (longer-term) -- direct publish to an MQTT broker if WiFi is available.

   The architecture must make it straightforward to add these without restructuring. They are all "sinks" for measurement data and optionally for log messages.

4. **Wire publishing of logs**: It should be possible (as an option) to send log messages over the network too, not just sensor data. This enables remote log viewing in the future.

5. **Remote config** is a pipe dream and out of scope. Do not design for it. Just don't paint yourself into a corner.

#### Design direction

Create a **`Sink`** interface (or similar name) in a new package (e.g. `internal/sink` or `internal/output`). A Sink represents any destination for data. The manager (or a new coordinator) fans out data to registered sinks.

Suggested interface shape (refine as you see fit):

```go
// Sink receives measurement batches and log entries for output.
type Sink interface {
    // Name identifies this sink for diagnostics (e.g. "sd", "notecard", "lora").
    Name() string

    // WriteMeasurements writes a batch of sensor measurements.
    // Implementations decide format (CSV to SD, JSON to Notecard, etc.).
    WriteMeasurements(t time.Time, device string, ms []sensors.Measurement) error

    // WriteLog writes a single log entry. Implementations may choose to
    // ignore logs (e.g. a LoRa sink might only send measurements).
    WriteLog(t time.Time, level log.Level, msg string) error

    // Flush forces any buffered data to be written. Called before sleep.
    Flush() error
}
```

Think about:

- The **`log.Logger`** should become a Sink (or wrap one). Serial output is just another sink. The logger's current `io.Writer` approach maps to a "serial sink" that formats text lines. The logger could hold `[]Sink` and fan out, or the manager could own the sink list and the logger stays simple (just serial). Argue your choice.
- **Separation of concerns**: The manager currently calls `m.slog()` for both debug logs and measurement output. With sinks, measurement data should go through `WriteMeasurements` and operational logs through `WriteLog`. The manager should iterate over sinks for each.
- **Error handling in sinks**: A failing sink (e.g. SD card full) should not block other sinks or crash the system. The manager should log sink errors to the remaining working sinks. Consider whether a sink should self-disable (like the logger change in Task 3) or whether the manager should handle retries/disabling.
- **`Flush` before sleep**: Before entering standby, the manager must flush all sinks so that buffered SD card writes hit the FAT and network sinks transmit their payloads.

Deliverables for this task:

- Create `internal/sink/sink.go` with the `Sink` interface and any shared types.
- Create skeleton files for future sink implementations: `internal/sink/sd/sd.go` (SD card sink, stubbed with TODOs), `internal/sink/serial/serial.go` (wraps the current serial logger behavior as a sink).
- Add doc comments on each explaining what the implementation will do.
- Modify `internal/manager/manager.go`: add a `sinks []sink.Sink` field to `Manager`. In `doSample`, after measuring, call `WriteMeasurements` on each sink. Replace or supplement `slog` calls to also call `WriteLog` on each sink. Add a `flush` call before sleep. **Keep the existing `log.Logger` and serial output working** -- the serial sink is just one of the sinks. Leave TODOs where the implementation is stubbed.
- Update `internal/manager/manager_test.go` to use a mock sink and verify that `WriteMeasurements` and `WriteLog` are called appropriately during `step()`.
- Update `targets/hypnos-m0/main.go` to show how sinks would be wired in (e.g. create a serial sink, optionally create an SD sink, pass them to the manager). Stub with TODOs for the SD sink construction.

Do NOT implement actual SD card I/O, file systems, or network drivers. This is architecture and skeleton only.

---

### Ordering

Execute tasks in this order: **1 → 2 → 3 → 4**. Task 4 modifies manager and main, so finish the earlier tasks first to avoid merge conflicts in your own work.

### Style

- Match the existing code style: minimal, no unnecessary abstractions, concise doc comments.
- This is a TinyGo project targeting Cortex-M0+ with 32KB RAM. Avoid heap-heavy patterns (no `fmt`, no `reflect`, minimize interface boxing and closures). Prefer fixed buffers and append-style building.
- Keep packages small and focused.
- Run `make test` after each task and fix any failures before moving on.
