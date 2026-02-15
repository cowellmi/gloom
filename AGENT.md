# Agent Guide

## Project Overview

Gloom is a portable, low-power IoT firmware written in TinyGo. It sleeps between sensor readings, wakes on interrupt, samples sensors, and logs measurements. The architecture is interface-driven so it can support **any combination of MCU, board, sensor, and output sink** — not just the current hardware. The Feather M0 + Hypnos board is simply the first target.

### Architecture Philosophy

Every hardware dependency hides behind an interface. Adding a new MCU, board, or sensor means writing a new implementation — never changing the core. The key seams are:

- `mcu.MCU` — chip-level sleep, wake-source config, USB. Currently: SAMD21. Could be: nRF52, ESP32, STM32, etc.
- `hal.System` — composable struct that assembles optional RTC (`hal.RTC`), power manager (`hal.PowerManager`), and MCU processor (`hal.Processor`) into a unified sleep/wake platform. Nil components degrade gracefully.
- `hal.RTC` — real-time clock interface (read time, set/clear wake alarm). Currently: DS3231 (`internal/rtc/ds3231.go`). Could be: PCF8523, etc.
- `hal.PowerManager` — board-level power rail control. Currently: Hypnos (`internal/power/hypnos.go`). Could be: Adalogger, custom carriers, etc.
- `sensor.Device` — Init / Name / Measure. Each sensor lives in its own sub-package.
- `sensor.Recorder` / `log.Sink` — output destinations (serial, SD file, MQTT, LoRa, etc.).

The firmware entry point lives in `cmd/gloom/` with a single generic `main.go` that auto-probes hardware. Board-specific code (UART setup, MCU init, default pin config) is separated into build-tagged files (e.g. `main_feather_m0.go`). Shared logic in `internal/` stays target-agnostic.

### Current Hardware (Hypnos + Feather M0)

- **MCU:** Adafruit Feather M0 (ATSAMD21 Cortex-M0, 32 KB RAM)
- **Board:** Hypnos FeatherWing — custom OPEnS Lab PCB with:
  - Two MOSFETs controlling 3.3 V (pin 5, active-low) and 5 V (pin 6) power rails
  - DS3231 RTC with two alarms and SQW output on pin 12
  - SD card reader (CS pin 11 on Hypnos 3.3, pin 10 on 3.2) — auto-detected via config defaults
- **Sleep mode:** SAMD21 STANDBY (`SCR.SLEEPDEEP = 1` then `WFI`). Before entry the firmware must disable enough clocks/peripherals to avoid overloading the voltage regulator (datasheet §16, p. 124).

### Sleep / Wake Cycle (happy path with RTC + PowerManager)

**Setup (once):**
1. Configure I2C, instantiate power manager (initial power cycle), probe RTC.
2. Build `hal.System` from proc, RTC, and power manager.
3. Probe SD card using config CS pins.
4. Load config, resolve sensors, create logger + sinks + manager.

**Loop (each cycle):**
1. Flush all sinks (serial, SD).
2. Set DS3231 alarm for next sample/heartbeat time.
3. Arm MCU wake pin, cut power rails, enter STANDBY.
4. _...CPU halted until alarm fires on pin 12..._
5. Clear alarm, restore power rails, wait for voltage stabilisation.
6. Read RTC time, push to logger.
7. Init each sensor, measure, fan out to recorders.
8. GC, loop.

Without an RTC or power manager, `hal.System.Sleep()` degrades to idle busy-wait using `time.Now()`.

## Repository Layout

```
cmd/gloom/
  main.go              Generic boot: config-driven probe, sinks, manager
  main_feather_m0.go   //go:build feather_m0 — initMCU(), boardDefaults(), UART0
  registry.go          Sensor registry (universal)
  justfile             Build/flash commands (board variable, default feather-m0)
internal/
  hal/                 System struct + RTC/PowerManager/Processor interfaces
  rtc/                 RTC implementations (DS3231 wrapper + probe)
  power/               PowerManager implementations (Hypnos rail control)
  sdcard/              Board-agnostic SD card + FAT filesystem wrapper
  wait/                Scheduler-free busy-wait delay (target-agnostic)
  debug/               Global debug logger backed by io.Writer (target-agnostic)
  mcu/                 MCU interface (target-agnostic)
  mcu/samd21/          SAMD21 impl: standby, USB detach/reattach, GCLK config
  manager/             Wake/sleep loop, sensor sampling, recorder fan-out (target-agnostic)
  config/              key=value config parser + DefaultINI template (target-agnostic)
  log/                 Leveled logger with per-sink filtering (target-agnostic)
  sensor/              Device + Recorder interfaces (target-agnostic)
  sensor/fake/         Dummy sensor for debugging
  sink/serial/         Serial text output (log.Sink + sensor.Recorder)
  sink/file/           Daily-rotating file output to data/ and logs/ (log.Sink + sensor.Recorder)
```

Packages marked *target-agnostic* contain no hardware imports and are testable with the standard Go toolchain. Hardware-specific code lives in `mcu/<chip>`, `rtc/`, `power/`, and the build-tagged board files in `cmd/gloom/`.

## Build & Test

```sh
# Run pure-Go unit tests (no hardware required)
just test

# Vet pure-Go packages
just vet

# Build firmware binary for feather-m0 (requires tinygo)
just build

# Flash firmware (requires bossac)
just flash

# Build for a different board (same MCU family)
just -f cmd/gloom/justfile build board=feather-m0-express

# Open serial monitor (requires tio)
just monitor
```

`test_pkgs` in the root justfile lists pure-Go packages testable with `go test`. Hardware-dependent packages (`rtc/`, `power/`, `mcu/samd21`) are only buildable with TinyGo and have no host-side tests.

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
- Use stack-allocated `[N]byte` scratch buffers passed as `buf[:0]` to avoid per-call allocations (see `recorderBufSize` in manager, `logBufSize` in logger).
- Call `runtime.GC()` once per wake cycle to collect per-cycle garbage.
- Log heap stats (`runtime.MemStats`) each cycle for monitoring.

### Interfaces & Layering

- `hal.System` — composable struct that assembles optional `hal.RTC`, `hal.PowerManager`, and `hal.Processor` into a unified sleep/wake platform. Constructed with `hal.NewSystem(proc, rtc, pm)`. RTC and PowerManager can be nil for graceful degradation (no RTC = no deep sleep, no PowerManager = no rail control).
- `hal.RTC` — real-time clock interface. Implementations live in `internal/rtc/`. Currently: `rtc.DS3231`.
- `hal.PowerManager` — power rail control interface. Implementations live in `internal/power/`. Currently: `power.Hypnos`.
- `hal.Processor` — hal-local interface for MCU operations needed by System (ArmWake, DisarmWake, Standby, PetWatchdog, Identifier). Satisfied by any `mcu.MCU` implementation.
- `mcu.MCU` — chip-level interface (superset of `hal.Processor`, adds EnableWatchdog). SAMD21 is the only impl today. A new chip adds `mcu/<chip>/`.
- `sensor.Device` — Init / Name / Measure. Each sensor gets its own sub-package under `sensor/`. Registration lives in `cmd/gloom/registry.go`.
- `sensor.Recorder` — receives measurement batches for output (serial text, CSV file, etc.). New output formats add a new `sink/<name>/` package.
- `log.Sink` — receives log entries. Serial and file sinks implement both `Recorder` and `Sink`.
- The manager (`internal/manager/`) knows nothing about specific hardware — it depends only on a local `system` interface (satisfied by `*hal.System`), `sensor.Device`, `sensor.Recorder`, and `log.Logger`.

### Adding a New Board

1. If the board uses a new MCU chip, create `internal/mcu/<chip>/` implementing the `mcu.MCU` interface.
2. If the board has a new RTC chip, create `internal/rtc/<chip>.go` implementing `hal.RTC`.
3. If the board has power rail control, create `internal/power/<board>.go` implementing `hal.PowerManager`, and register it in the `switch cfg.Power` block in `main.go`.
4. Add `cmd/gloom/main_<board>.go` with a `//go:build <board_tag>` constraint. It must provide:
   - `initMCU() mcu.MCU` — chip-specific init (UART, watchdog)
   - `boardDefaults(cfg *config.Config)` — default pin numbers, power manager name
   - `debugWriter() *machine.UART` — for serial sinks
5. The generic `main.go`, sensor registry, and all `internal/` logic stay untouched.
6. Build with `tinygo build -target=<board> ./cmd/gloom/` — TinyGo's build tags select the right board file automatically.

### Style

- No `fmt` package — it's too large for TinyGo on constrained targets. Use `strconv`, manual `append`, or `debug.Log` for diagnostic output (see Debugging section below). Do not use `println` — it routes to USB-CDC which is unreliable on this target.
- String building via `append(buf, ...)` chains.
- Prefer returning errors over panicking.
- Short, descriptive variable names (`cfg`, `proc`, `sys`, `buf`, `ms`).
- Comments on exported types and functions (Go doc style).

### Hypnos / SAMD21 Hardware Notes

These are specific to the current target and live in `rtc/`, `power/`, and `mcu/samd21/`:

- Pin 12 is the DS3231 alarm interrupt line (active-low, needs pullup).
- 3.3 V rail (pin 5) is **active-low** — `Low()` = on, `High()` = off.
- After powering on rails, wait `powerOnDelay` (2 s) for voltage to stabilize before talking to sensors.
- Before STANDBY: detach USB, disable SysTick (prevents a known SAMD21 lock-up). After wake: re-enable SysTick, re-attach USB.
- GCLK_EIC must be rerouted to GCLK6 (OSCULP32K, run-in-standby) so edge-detection works during STANDBY sleep. `prepareStandby()` (called internally by `ArmWake`) handles this and is idempotent.
- Flash sleep-power-reduction errata: SLEEPPRM must be set to DISABLED on some SAMD21 revisions.
- **Do not use `time.Sleep`** — it goes through TinyGo's scheduler/SysTick path, which is opaque and has shown unreliable behavior after SAMD21 standby wake. Use `wait.For(d)` from `internal/wait` everywhere instead. It busy-waits on `time.Now()` / `time.Since()` using the monotonic clock, which survives standby. There are no other goroutines on these devices, so spinning has zero downside.
- UART0 on SERCOM0 (D0/D1) is manually configured in `cmd/gloom/main_feather_m0.go` because TinyGo's Feather M0 board file only exposes UART1 on SERCOM1 (D10/D11), which conflicts with SD card CS pins. RX interrupts are not enabled to avoid conflicting with TinyGo's compile-time IRQ_SERCOM0 handler.

### Debugging

All diagnostic output goes through **`debug.Log`** from `internal/debug`, which writes to a global `io.Writer`. The board-specific init file (e.g. `main_feather_m0.go`) wires `debug.W = UART0` early in `initMCU()`, so messages appear on the hardware UART serial monitor.

- **Use `debug.Log("msg")` for any debug/diagnostic output.** It is a no-op when `debug.W` is nil, so it is safe to call from any package at any time.
- **Do not use `println`.** TinyGo's `println` routes to USB-CDC on the Feather M0, not to the hardware UART. The TinyGo `-serial=uart` flag cannot be used because it targets UART1 (SERCOM1 / D10/D11), which conflicts with the Hypnos SD card CS pins.
- **Panics and stack traces** still go to USB-CDC (`println` path). To see them, connect a USB cable and open a CDC serial monitor in addition to the UART monitor.

To view UART output, connect a USB-to-serial adapter to D0/D1 and open a monitor at 115200 baud:

```sh
just monitor
```
