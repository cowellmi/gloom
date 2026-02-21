# Agent Guide

## Project Overview

Gloom is a portable, low-power IoT firmware written in TinyGo. It sleeps between sensor readings, wakes on interrupt, samples sensors, and logs measurements. The architecture is interface-driven so it can support **any combination of MCU, board, sensor, and output sink** — not just the current hardware. The Feather M0 + Hypnos board is simply the first target.

### Architecture Philosophy

Every hardware dependency hides behind an interface. Adding a new MCU, board, or sensor means writing a new implementation — never changing the core. The key seams are:

- `hal.MCU` — chip-level sleep, wake-source config, watchdog. Currently: SAMD21. Could be: nRF52, ESP32, STM32, etc.
- `hal.System` — composable struct that assembles optional RTC (`hal.RTC`), power rails (`hal.Rails`), and MCU (`hal.MCU`) into a unified sleep/wake platform. Manages N deadline slots (one per config group) and external interrupt pins. Nil components degrade gracefully.
- `hal.RTC` — real-time clock interface (read time, set/clear wake alarm). Currently: DS3231 (`internal/drivers/ds3231/ds3231.go`). Could be: PCF8523, etc.
- `hal.Rails` — board-level power rail control. Currently: generic `power.Controller` (`internal/power/power.go`). Each rail carries a `WakeReason` bitmask (`WakeAlways` for core, `WakeSensors` for sensor power). Power rails are a compile-time board decision via build-tagged `boardPower()` functions (e.g. `board_hypnos.go`). Builds with `no_hypnos` tag skip rail control.
- `sensor.Device` — Init / Name / Measure. Each sensor lives in its own sub-package.
- `sensor.Recorder` / `log.Sink` — output destinations (serial, SD file, MQTT, LoRa, etc.).

The firmware entry point lives in `cmd/gloom/` with a single generic `main.go` that auto-probes hardware. Board-specific code (UART setup, MCU init, default pin config) is separated into build-tagged files (e.g. `main_feather_m0.go`). Shared logic in `internal/` stays target-agnostic.

### Configuration Model

Configuration uses an INI-style format with two section types:

- **`[device]`** — hardware settings: log sinks (with per-sink log level), data sinks, LED, pin overrides. Not per-group. Data sinks are device-wide — all groups share the same set.
- **`[group-name]`** — any other section defines a group. A group fires on a timer (`interval`), an external interrupt (`external_int_pin`), or both. Each group has its own sensors, host, and payload profile.

Log sinks use `name:level` syntax (e.g. `uart:debug`, `sd:error`). Data sinks are plain names. See `example.config.ini` for the full spec.

Payload profiles (`none`, `min`, `full`) define predefined health-check POST bodies for remote monitoring. Sensor measurements are NOT part of payloads — data flows to servers via data sinks (e.g. Blues Notecard).

### Current Hardware (Hypnos + Feather M0)

- **MCU:** Adafruit Feather M0 (ATSAMD21 Cortex-M0, 32 KB RAM)
- **Board:** Hypnos FeatherWing — custom OPEnS Lab PCB with:
  - Two MOSFETs controlling 3.3 V (pin 5, active-low) and 5 V (pin 6) power rails
  - DS3231 RTC with two alarms and SQW output on pin 12
  - SD card reader (CS pin 11 on Hypnos 3.3, pin 10 on 3.2) — auto-detected via config defaults
- **Sleep mode:** SAMD21 STANDBY (`SCR.SLEEPDEEP = 1` then `WFI`). Before entry the firmware must disable enough clocks/peripherals to avoid overloading the voltage regulator (datasheet §16, p. 124).

### Sleep / Wake Cycle (happy path with RTC + Rails)

**Setup (once):**
1. Instantiate power rails via `boardPower()` (initial power cycle + stabilise delay).
2. Configure I2C (after rails are up so peripherals don't pull bus low).
3. Probe RTC, probe SD card using config CS pins.
4. Load config from SD, build sinks, resolve groups, build `hal.System` + manager.

**Loop (each cycle):**
1. Flush all sinks (serial, SD).
2. Set DS3231 alarm for earliest group deadline.
3. Arm MCU wake pins (RTC + any external interrupt pins), cut power rails, enter STANDBY.
4. _...CPU halted until alarm or external interrupt fires..._
5. Clear alarm, restore core power rails.
6. Read RTC time, push to logger.
7. Resolve which groups fired (deadline-based and/or external interrupt).
8. If any fired group has sensors, enable sensor power rails.
9. For each fired group: init sensors, measure, fan out to device-wide recorders; POST payload if configured.
10. GC, loop.

Without an RTC or power rails, `hal.System.Sleep()` degrades to idle busy-wait using `time.Now()`.

## Repository Layout

```
cmd/gloom/
  main.go              Generic boot: config-driven probe, sinks, group resolution, manager
  main_feather_m0.go   //go:build feather_m0 — initBoard(), UART0, pin defaults
  board_hypnos.go      //go:build feather_m0 && !no_hypnos — boardPower() returns Hypnos D5/D6 rails
  board_no_hypnos.go   //go:build feather_m0 && no_hypnos — boardPower() returns nil (no rail control)
  registry.go          Sensor registry (universal)
  justfile             Build/flash commands (board variable, default feather-m0)
internal/
  hal/                 System struct + MCU/RTC/Rails interfaces
  drivers/ds3231/      DS3231 hal.RTC implementation (probe + wake-alarm)
  drivers/pcf8523/     PCF8523 hal.RTC implementation (countdown timer wake)
  power/               Generic Rails (GPIO rail control)
  sdcard/              Board-agnostic SD card + FAT filesystem wrapper
  wait/                Scheduler-free busy-wait delay (target-agnostic)
  debug/               Global debug logger backed by io.Writer (target-agnostic)
  targets/             Parent directory for chip-specific MCU implementations
  targets/samd21/      SAMD21 impl: standby, USB detach/reattach, GCLK config
  manager/             Wake/sleep loop, group execution, recorder fan-out (target-agnostic)
  config/              INI section parser, Config/Device/Group types, DefaultINI (target-agnostic)
  log/                 Leveled logger with per-sink filtering (target-agnostic)
  sensor/              Device + Recorder interfaces (target-agnostic)
  sensor/fake/         Dummy sensor for debugging
  sink/serial/         Serial text output (log.Sink + sensor.Recorder)
  sink/file/           Daily-rotating file output to data/ and logs/ (log.Sink + sensor.Recorder)
```

Packages marked *target-agnostic* contain no hardware imports and are testable with the standard Go toolchain. Hardware-specific code lives in `targets/<chip>`, `drivers/<chip>`, `power/`, and the build-tagged board files in `cmd/gloom/`.

## Build & Test

The project uses a nix flake to provide the TinyGo toolchain. All build commands that require TinyGo must be run through `nix develop`:

```sh
# Build firmware (TinyGo via nix)
nix develop --command make

# Run pure-Go unit tests (no hardware required)
nix develop --command make test

# Vet pure-Go packages
nix develop --command make vet

# Flash firmware (requires bossac)
nix develop --command make flash

# Open serial monitor (requires tio)
nix develop --command make monitor
```

Do **not** use `go build` or `tinygo build` directly — the nix flake ensures the correct TinyGo version and toolchain are available.

`TEST_PKGS` in the Makefile lists pure-Go packages testable with `go test`. Hardware-dependent packages (`drivers/ds3231`, `power/`, `targets/samd21`) are only buildable with TinyGo and have no host-side tests.

## Coding Conventions

### Language & Toolchain

- Go 1.24, compiled with TinyGo for the target board (currently `feather-m0`).
- Module path: `github.com/cowellmi/gloom`.
- Dependencies: `tinygo.org/x/drivers`, `tinygo.org/x/tinyfs`.

### Error Handling

**Every error must be handled explicitly.** This is the single most important rule.

- **Check every error return.** If a function returns an error, the caller must inspect it — no bare calls to functions that return errors.
- **If you intentionally discard an error, assign it to `_` and add a comment explaining why.** For example:

```go
// ClearWake returns an error only when I2C fails, which is
// non-fatal at this point since we've already woken up.
_ = s.rtc.ClearWake()
```

- **Never** write `mightFail()` when `mightFail` returns an error. Always write either:
  - `if err := mightFail(); err != nil { ... }` (handle it), or
  - `_ = mightFail() // reason this error is safe to ignore`
- Collect non-fatal init errors into a slice and log them once the logger is available (see `main.go` pattern with `initErrs`).
- Sinks self-disable on write error (set writer to nil). The logger silently drops entries to broken sinks — this is intentional so a failed SD card doesn't crash the device.

### Memory & Allocations

- The current MCU has only 32 KB RAM. Minimize heap allocations even if a future target has more memory — the core code must remain viable on constrained devices.
- **Scratch buffers are owned by the component that uses them.** Each sink, recorder, or package that needs formatting scratch space declares its own `[N]byte` field (struct) or package-level var, sized to fit its workload. Callers never pass buffers through interfaces — the buffer is an internal optimization detail, not part of the API contract. Use `s.buf[:0]` (struct field) or `buf[:0]` (package var) and build output via `append` chains.
- Never allocate in a formatting hot path. Avoid `string(buf[:])` returns that escape stack arrays to the heap — prefer `appendX(buf, ...)` helpers that append directly into the caller's scratch buffer.
- Call `runtime.GC()` once per wake cycle to collect per-cycle garbage.
- Log heap stats (`runtime.MemStats`) each cycle for monitoring.

### Interfaces & Layering

**The `hal` package is the source of truth for all hardware interfaces.** `hal.MCU`, `hal.RTC`, and `hal.Rails` define what hardware implementations must provide. Implementation packages (`targets/samd21`, `drivers/ds3231`, `power/`) satisfy these interfaces via Go structural typing — they do not import `hal` for the interface definition (except for shared types like `hal.WakeReason`). This keeps the dependency graph clean: `hal` depends on nothing, implementations depend on `hal` only for types, and the composition happens in `cmd/gloom/main.go`.

- `hal.System` — composable struct that assembles optional `hal.RTC`, `hal.Rails`, and `hal.MCU` into a unified sleep/wake platform. Constructed with `hal.NewSystem(mcu, rtc, rails, intervals)` where intervals is one duration per group (0 = no timer). RTC and Rails can be nil for graceful degradation. External interrupt pins are registered per group slot via `RegisterExternalPin(pin, slot)`. `Sleep()` returns `[]bool` indicating which group slots fired.
- `hal.RTC` — real-time clock interface. Implementations live in `internal/drivers/<chip>/`. Currently: `ds3231.RTC`, `pcf8523.RTC`.
- `hal.Rails` — power rail control interface. Implementation lives in `internal/power/`. `power.Controller` is a generic struct configured with GPIO rails and polarities. Core rails (`WakeAlways`) restore after every wake; sensor rails (`WakeSensors`) activate only when fired groups have sensors (via `System.EnableSensorRails()`).
- `hal.MCU` — chip-level interface for MCU operations (ArmWake, DisarmWake, Standby, EnableWatchdog, DisableWatchdog, PetWatchdog, Identifier). Defined in `hal/mcu.go`. SAMD21 is the only impl today. A new chip adds `targets/<chip>/`.
- `sensor.Device` — Init / Name / Measure. Each sensor gets its own sub-package under `sensor/`. Registration lives in `cmd/gloom/registry.go`.
- `sensor.Recorder` — receives measurement batches for output (serial text, CSV file, etc.). New output formats add a new `sink/<name>/` package.
- `log.Sink` — receives log entries. Serial and file sinks implement both `Recorder` and `Sink`.
- The manager (`internal/manager/`) knows nothing about specific hardware — it depends only on a local `system` interface (satisfied by `*hal.System`), resolved `Group` structs (sensors only), shared device-wide recorders, and `log.Logger`. The manager's `step()` iterates fired groups and calls `doGroup()` for each.

### Adding a New Board

1. If the board uses a new MCU chip, create `internal/targets/<chip>/` implementing `hal.MCU`.
2. If the board has a new RTC chip, create `internal/drivers/<chip>/<chip>.go` implementing `hal.RTC`.
3. If the board has power rail control, add a build-tagged `board_<name>.go` in `cmd/gloom/` that provides `boardPower() []power.Rail`. The generic `power.Controller` handles any combination of GPIO rails with `WakeReason` bitmasks. Hypnos is the default for `feather_m0`; pass `-tags no_hypnos` to build without rail control.
4. Add `cmd/gloom/main_<board>.go` with a `//go:build <board_tag>` constraint. It must provide:
   - `initBoard(cfg *config.Config) Board` — configure MCU, UART, USB-CDC, I2C, SPI, and apply board-specific pin defaults to `cfg.Device`.
5. The generic `main.go`, sensor registry, and all `internal/` logic stay untouched.
6. Build with `tinygo build -target=<board> ./cmd/gloom/` — TinyGo's build tags select the right board file automatically.

### Style

- No `fmt` package — it's too large for TinyGo on constrained targets. Use `strconv` and manual `append` for string building. Do not use `println` — it routes to USB-CDC which is unreliable on this target.
- String building via `append(buf, ...)` chains.
- Prefer returning errors over panicking.
- Short, descriptive variable names (`cfg`, `proc`, `sys`, `buf`, `ms`).
- Comments on exported types and functions (Go doc style).

### Hypnos / SAMD21 Hardware Notes

These are specific to the current target and live in `drivers/ds3231/`, `power/`, and `targets/samd21/`:

- Pin 12 is the DS3231 alarm interrupt line (active-low, needs pullup).
- 3.3 V rail (pin 5) is **active-low** — `Low()` = on, `High()` = off. Configured at compile time via `board_hypnos.go` (build tag: `feather_m0 && !no_hypnos`).
- After powering on rails, wait `powerOnDelay` (2 s) for voltage to stabilize before talking to sensors.
- Before STANDBY: detach USB, disable SysTick (prevents a known SAMD21 lock-up). After wake: re-enable SysTick, re-attach USB.
- GCLK_EIC must be rerouted to GCLK6 (OSCULP32K, run-in-standby) so edge-detection works during STANDBY sleep. `prepareStandby()` (called internally by `ArmWake`) handles this and is idempotent.
- Flash sleep-power-reduction errata: SLEEPPRM must be set to DISABLED on some SAMD21 revisions.
- **Do not use `time.Sleep`** — it goes through TinyGo's scheduler/SysTick path, which is opaque and has shown unreliable behavior after SAMD21 standby wake. Use `wait.For(d)` from `internal/wait` everywhere instead. It busy-waits on `time.Now()` / `time.Since()` using the monotonic clock, which survives standby. There are no other goroutines on these devices, so spinning has zero downside.
- UART0 on SERCOM0 (D0/D1) is manually configured in `cmd/gloom/main_feather_m0.go` because TinyGo's Feather M0 board file only exposes UART1 on SERCOM1 (D10/D11), which conflicts with SD card CS pins. RX interrupts are not enabled to avoid conflicting with TinyGo's compile-time IRQ_SERCOM0 handler.

### Naming

- **Constructors should include the type name**, not just `New`. Prefer `power.NewController()`, `sdcard.NewCard()`, `log.NewLogger()` over bare `power.New()`. This makes call sites readable without relying on the package name for context. Exception: very small single-type packages where `New` is unambiguous (e.g. `samd21.New()`).

### Debugging

The `internal/debug` package provides a global `debug.Log` backed by an `io.Writer`. The board-specific init file (e.g. `main_feather_m0.go`) wires `debug.W = UART0` early in `initBoard()`, so messages appear on the hardware UART serial monitor.

- **`debug.Log` is for local debugging only — do not commit calls to it.** Use it freely while developing, but remove all calls before committing. Committed diagnostic output should go through the structured logger (`log.Logger`) instead.
- **Do not use `println`.** TinyGo's `println` routes to USB-CDC on the Feather M0, not to the hardware UART. The TinyGo `-serial=uart` flag cannot be used because it targets UART1 (SERCOM1 / D10/D11), which conflicts with the Hypnos SD card CS pins.
- **Panics and stack traces** still go to USB-CDC (`println` path). To see them, connect a USB cable and open a CDC serial monitor in addition to the UART monitor.

To view UART output, connect a USB-to-serial adapter to D0/D1 and open a monitor at 115200 baud:

```sh
just monitor
```
