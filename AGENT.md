# Agent Guide

## Project Overview

Gloom is a portable, low-power IoT firmware written in TinyGo. It sleeps between sensor readings, wakes on interrupt, samples sensors, and logs measurements. The architecture is interface-driven so it can support **any combination of MCU, board, sensor, and output sink** — not just the current hardware. The Feather M0 + Hypnos board is simply the first target.

### Architecture Philosophy

Every hardware dependency hides behind an interface. Adding a new MCU, board, or sensor means writing a new implementation — never changing the core. The key seams are:

- `mcu.MCU` — chip-level sleep, wake-source config, USB. Currently: SAMD21. Could be: nRF52, ESP32, STM32, etc.
- `hal.Platform` — board-level capabilities (clock, sleep orchestration, power rails). Currently: Hypnos. Could be: Adalogger, custom carrier boards, etc.
- `sensor.Device` — Init / Name / Measure. Each sensor lives in its own sub-package.
- `sensor.Recorder` / `log.Sink` — output destinations (serial, SD file, MQTT, LoRa, etc.).

New targets get their own directory under `targets/` with a `main.go` that wires the right MCU, board, sensors, and sinks together. Shared logic in `internal/` stays target-agnostic.

### Current Hardware (Hypnos + Feather M0)

- **MCU:** Adafruit Feather M0 (ATSAMD21 Cortex-M0, 32 KB RAM)
- **Board:** Hypnos FeatherWing — custom OPEnS Lab PCB with:
  - Two MOSFETs controlling 3.3 V (pin 5, active-low) and 5 V (pin 6) power rails
  - DS3231 RTC with two alarms and SQW output on pin 12
  - SD card reader (CS pin 11 on Hypnos 3.3, pin 10 on 3.2) — auto-detected during Probe
- **Sleep mode:** SAMD21 STANDBY (`SCR.SLEEPDEEP = 1` then `WFI`). Before entry the firmware must disable enough clocks/peripherals to avoid overloading the voltage regulator (datasheet §16, p. 124).

### Sleep / Wake Cycle (Hypnos happy path)

**Setup (once):**
1. Configure I2C, probe Hypnos (RTC init, clear alarms, SQW off).
2. Configure power-rail pins (5, 6) as OUTPUT; turn rails on.
3. Configure LED pin 13 as OUTPUT.
4. Load config, resolve sensors, create logger + sinks + manager.

**Loop (each cycle):**
1. Flush all sinks (serial, SD).
2. Set DS3231 alarm 1 for next sample time.
3. Power off rails, detach USB, disable SysTick, enter STANDBY.
4. _...CPU halted until alarm fires on pin 12..._
5. Re-enable SysTick, re-attach USB.
6. Power on rails, wait for RTC, clear alarms.
7. Read RTC time, push to logger.
8. Init each sensor, measure, fan out to recorders.
9. GC, loop.

Other boards would implement `hal.Platform.Sleep()` differently (e.g. timer-based wake on an nRF52) while the manager loop stays the same.

## Repository Layout

```
targets/
  feather-m0/          First target: Feather M0 + Hypnos (main.go, registry, uart0, justfile)
internal/
  hal/                Platform interface + Fallback impl (target-agnostic)
  boards/hypnos/      Hypnos Board (Platform impl): RTC, rails, standby, SD card
  sdcard/             Board-agnostic SD card + FAT filesystem wrapper
  wait/               Scheduler-free busy-wait delay (target-agnostic)
  debug/              Global debug logger backed by io.Writer (target-agnostic)
  mcu/                MCU interface (target-agnostic)
  mcu/samd21/         SAMD21 impl: standby, USB detach/reattach, GCLK config
  manager/            Wake/sleep loop, sensor sampling, recorder fan-out (target-agnostic)
  config/             key=value config parser + DefaultINI template (target-agnostic)
  log/                Leveled logger with per-sink filtering (target-agnostic)
  sensor/             Device + Recorder interfaces (target-agnostic)
  sensor/fake/        Dummy sensor for debugging
  sink/serial/        Serial text output (log.Sink + sensor.Recorder)
  sink/file/          Daily-rotating file output to data/ and logs/ (log.Sink + sensor.Recorder)
```

Packages marked *target-agnostic* contain no hardware imports and are testable with the standard Go toolchain. Hardware-specific code lives exclusively in `mcu/<chip>`, `boards/<board>`, and `targets/<target>`.

## Build & Test

```sh
# Run pure-Go unit tests (no hardware required)
just test

# Vet pure-Go packages
just vet

# Build firmware binary (requires tinygo)
just -f targets/feather-m0/justfile build

# Flash firmware (requires bossac)
just -f targets/feather-m0/justfile flash

# Open serial monitor (requires tio)
just monitor
```

`test_pkgs` in the root justfile lists pure-Go packages testable with `go test`. Hardware-dependent packages (`boards/hypnos`, `mcu/samd21`) are only buildable with TinyGo and have no host-side tests.

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
// SetInterrupt returns an error only when the pin/channel is invalid,
// which cannot happen here because AlarmPin is a compile-time constant.
_ = AlarmPin.SetInterrupt(0, nil)
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

- `hal.Platform` — abstracts board-level capabilities (clock, sleep). Hypnos `Board` is the primary impl. The feather-m0 target treats Probe failure as fatal (blink + halt). `hal.Fallback` exists for targets that want degraded-mode operation using `time.Now()` / `time.Sleep()`. A new board (e.g. Adalogger, custom carrier) would add a new `boards/<name>/` package implementing this interface.
- `mcu.MCU` — abstracts chip-level standby, wake-source config, USB. SAMD21 is the only impl today. A new chip would add `mcu/<chip>/`. The MCU is injected into the board at probe time.
- `sensor.Device` — Init / Name / Measure. Each sensor gets its own sub-package under `sensor/`. Registration is per-target via a `sensorRegistry` map in the target's package.
- `sensor.Recorder` — receives measurement batches for output (serial text, CSV file, etc.). New output formats add a new `sink/<name>/` package.
- `log.Sink` — receives log entries. Serial and file sinks implement both `Recorder` and `Sink`.
- The manager (`internal/manager/`) knows nothing about specific hardware — it depends only on `hal.Platform`, `sensor.Device`, `sensor.Recorder`, and `log.Logger`.

### Adding a New Target

1. Create `targets/<name>/` with its own `main.go` and `Makefile`.
2. Wire the appropriate `mcu.MCU` and `hal.Platform` implementations.
3. Populate a `sensorRegistry` with the sensors available on that hardware.
4. Register sinks and recorders. The manager and all `internal/` logic stay untouched.

### Style

- No `fmt` package — it's too large for TinyGo on constrained targets. Use `strconv`, manual `append`, or `debug.Log` for diagnostic output (see Debugging section below). Do not use `println` — it routes to USB-CDC which is unreliable on this target.
- String building via `append(buf, ...)` chains.
- Prefer returning errors over panicking.
- Short, descriptive variable names (`cfg`, `proc`, `sys`, `buf`, `ms`).
- Comments on exported types and functions (Go doc style).

### Hypnos / SAMD21 Hardware Notes

These are specific to the current target and live in `boards/hypnos/` and `mcu/samd21/`:

- Pin 12 is the DS3231 alarm interrupt line (active-low, needs pullup).
- 3.3 V rail (pin 5) is **active-low** — `Low()` = on, `High()` = off.
- After powering on rails, wait `powerOnDelay` (2 s) for voltage to stabilize before talking to sensors.
- Before STANDBY: detach USB, disable SysTick (prevents a known SAMD21 lock-up). After wake: re-enable SysTick, re-attach USB.
- GCLK_EIC must be rerouted to GCLK6 (OSCULP32K, run-in-standby) so edge-detection works during STANDBY sleep. `PrepareStandby()` handles this and is idempotent.
- Flash sleep-power-reduction errata: SLEEPPRM must be set to DISABLED on some SAMD21 revisions.
- **Do not use `time.Sleep`** — it goes through TinyGo's scheduler/SysTick path, which is opaque and has shown unreliable behavior after SAMD21 standby wake. Use `wait.For(d)` from `internal/wait` everywhere instead. It busy-waits on `time.Now()` / `time.Since()` using the monotonic clock, which survives standby. There are no other goroutines on these devices, so spinning has zero downside.
- UART0 on SERCOM0 (D0/D1) is manually configured in `targets/feather-m0/uart0.go` because TinyGo's Feather M0 board file only exposes UART1 on SERCOM1 (D10/D11), which conflicts with SD card CS pins. RX interrupts are not enabled to avoid conflicting with TinyGo's compile-time IRQ_SERCOM0 handler.

### Debugging

All diagnostic output goes through **`debug.Log`** from `internal/debug`, which writes to a global `io.Writer`. In `main.go`, this is wired to the custom UART0 early in startup (`debug.W = UART0`), so messages appear on the hardware UART serial monitor.

- **Use `debug.Log("msg")` for any debug/diagnostic output.** It is a no-op when `debug.W` is nil, so it is safe to call from any package at any time.
- **Do not use `println`.** TinyGo's `println` routes to USB-CDC on the Feather M0, not to the hardware UART. The TinyGo `-serial=uart` flag cannot be used because it targets UART1 (SERCOM1 / D10/D11), which conflicts with the Hypnos SD card CS pins.
- **Panics and stack traces** still go to USB-CDC (`println` path). To see them, connect a USB cable and open a CDC serial monitor in addition to the UART monitor.

To view UART output, connect a USB-to-serial adapter to D0/D1 and open a monitor at 115200 baud:

```sh
just monitor
```
