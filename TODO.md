# TODO

Items are ordered by severity: critical (data loss / field failure) first, then high (correctness / memory), medium (conventions / maintainability), low (polish), and finally features.

---

## High

### Verify I2C bus recovery on Hypnos hardware

`RecoverI2C` has been implemented and confirmed to boot cleanly on a bare Feather M0 (no peripherals), but the stuck-bus recovery path has not been exercised with a real DS3231. To test: run a tight `ReadTime()` loop on Hypnos, trigger a mid-transaction reset (reset button or watchdog timeout), and confirm the device boots cleanly and probes the DS3231 on the next cycle.

---

## Medium

### Config boolean values not documented for non-programmers

In `internal/config/config.go`, boolean fields like `serial` and `enable_led` only accept the exact string `"true"` to enable. Any other value (including `"yes"`, `"1"`, `"TRUE"`) silently means false. For a framework targeting non-programmers, either document the accepted values clearly in a sample config file, or accept common truthy variants (`"true"`, `"yes"`, `"1"`, case-insensitive).

### Build-tag `machine` imports for standard Go testability

Several packages (`internal/rtc/ds3231`, `internal/mcu/samd21`, `internal/power`, `internal/sdcard`, `cmd/gloom`) import `machine`, which is a TinyGo-only pseudo-package. This means they can't be compiled or tested under the standard Go toolchain. Moving `machine`-dependent code into `//go:build tinygo` files with no-op stubs for `//go:build !tinygo` would allow unit testing, `go vet`, and IDE tooling to work on all packages without a TinyGo install.

---

## Low

### SD card probe may fail after watchdog reset on Hypnos

After a watchdog reset, the SD card can be stuck mid-SPI-command. `power.NewController` power-cycles the rails (250ms off, 2s stabilise) before any peripheral probing, which should discharge the card's capacitors and reset its state machine. If SD card probe failures are observed specifically after watchdog resets on Hypnos, the power-cycle timing in `internal/power/power.go` (`powerCycleDelay`) is the first place to adjust.

### No CSV header row in data files

Data files are written as bare CSV (`timestamp,device,label,value,unit`) but there's no header row. Adding a header on file creation would make the CSVs self-documenting for researchers working with the data offline.

---

## Features

### Configurable external wake pins

`hal.System` supports multiple wake pins via `AddWakePin(pin)`, but there's no way to configure them from `config.ini` yet. Add a `wake_pins` config key that accepts a comma-separated list of GPIO pin numbers. These pins are armed as falling-edge wake sources alongside the RTC alarm pin before each standby entry.

Primary use case: a tipping-bucket rain gauge with a reed switch that pulls a GPIO low on each tip. The device sleeps between sample intervals but wakes immediately on a tip event to record it with a precise timestamp. Other examples include pushbuttons for manual wake and sensor threshold interrupt lines.

Config example:
```
# GPIO pins that wake the device from deep sleep (comma-separated).
# Example: pin 7 connected to a tipping-bucket rain gauge reed switch.
wake_pins = 7
```

Implementation:
- Add `WakePins []uint8` to `config.Config` and parse `wake_pins` using the existing `parsePinList` helper.
- In `cmd/gloom/main.go`, after building the `hal.System`, call `sys.AddWakePin(pin)` for each configured pin.
- The `WakeReason` will resolve as `WakeExternal` for non-RTC wake sources. Callers can handle this in the manager's `step()` if tip-counting or event logging is needed.

### File retention / pruning for SD card logs and sensor data

Daily rotation is implemented with files organized into directories: `data/20260214.csv` for sensor recordings and `logs/20260214.log` for log entries. Old files are never deleted. Add configurable retention periods with separate settings for logs and sensor recordings:

- `log_retain_days` — number of days to keep log files (e.g. 7). Diagnostic logs are high-volume and low-value once reviewed; a researcher may want to keep only a few days.
- `data_retain_days` — number of days to keep sensor recordings. Sensor data is the primary scientific output and may need to be kept indefinitely (`0` = keep forever) until manually retrieved from the SD card.

Implementation notes:
- On startup or daily rotation, compute expected old filenames (`{dir}/{YYYYMMDD}{ext}`) going back beyond the retention window and call `card.Remove` for each. This avoids FAT directory listing (which is limited in fatfs) by using predictable date-stamped names.
- `card.Remove` already exists for this purpose.

### Adalogger FeatherWing as a `hal.Platform` implementation

Add the Adalogger FeatherWing (PCF8523 RTC + SD card) as a second board.

#### 1. `sensor.Sleeper` interface — `internal/sensor/sensor.go`

For boards without MOSFETs, add an opt-in software shutdown interface:

```go
type Sleeper interface {
    Sleep() error
}
```

- Sensors that support a low-power register implement `Sleeper`. The manager type-asserts each sensor before sleep and calls `Sleep()` if available.
- `Init()` already handles re-initialization on wake — no new wake method needed.
- Works on Hypnos too (software sleep before hard rail cut = cleaner shutdown).
- Passive analog sensors simply don't implement the interface.

Wire the `Sleeper` calls into `manager.doSleep()`, before `sys.Sleep()`.

#### 2. Board file — `cmd/gloom/main_adalogger_m0.go`

Add a build-tagged board file following the existing `main_feather_m0.go` pattern:

```go
//go:build adalogger_m0
```

Provides `initMCU() hal.MCU`, `boardDefaults(cfg *config.Config)`, and `debugWriter() *machine.UART`. The generic `main.go`, sensor registry, and all `internal/` logic stay untouched.

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

#### 7. Update config on env var updates from Blues Notehub

When we receive a message from Blues Notecard that their have been env vars updates for this device from Notehub, update the manager.cfg with the new values (or just do a hard reset?).

---

### Auto-probe with graceful degradation

The firmware entry point lives in `cmd/gloom/` with MCU-specific code separated via build tags. The single `main.go` auto-probes hardware in priority order and degrades gracefully at each step. The "target" is the MCU board (selected by `-target=` flag at build time); within it we discover which FeatherWings and peripherals are attached.

#### 1. Hypnos graceful degradation + Adalogger board

Work in this order:

**Step A — Hypnos graceful SD card failure.** Change `hypnos.Probe()` so SD card failure is non-fatal: return the board with `Card = nil` and log the error. RTC is still required for Hypnos to be considered "found." This unblocks the probe cascade — Hypnos can succeed as a platform even if its SD card is missing or corrupt.

**Step B — Adalogger board implementation** (`internal/boards/adalogger/`). PCF8523 RTC driver is done (`internal/rtc/pcf8523/`); remaining work is the `hal.Platform` board struct. The Adalogger FeatherWing uses SD card CS on pin 10 by default, but the CS pin should be configurable — add `sd_cs_pin` to config so users can wire an SD card reader to any GPIO. This works because config lives in Notehub (Blues Notecard env vars) and is pushed to the device over the air. The Notecard caches env vars locally, so they're available even when cellular is offline. On boot, config syncs down to SD card `config.ini` as a backup, giving graceful degradation when neither Notecard nor connectivity is available.

**Step C — Auto-probe cascade in `main.go`.** Boot sequence probes hardware in priority order, collecting non-fatal errors:

1. `initMCU()` — board + MCU + debug output (fatal on failure)
2. `machine.I2C0.Configure()` — I2C bus (fatal on failure)
3. Probe Notecard on I2C `0x17` → `nc` (nil if not found)
4. Load config: Notecard env vars → SD card `config.ini` → `config.Default()` (need config before board probe so `sd_cs_pin` is available)
5. Probe Hypnos → if found: `platform = hypnos.Board`, `card = board.Card` (may be nil)
6. Else probe Adalogger (using `sd_cs_pin` from config if set, else default pin 10) → if found: `platform = adalogger.Board`, `card = board.Card`
7. Else: `platform = hal.Fallback{}`, `card = nil`
8. Wire sinks/recorders with nil guards: file sink only if `card != nil`, notecard sink only if `nc != nil`
9. Serial UART always enabled by default for debugging

Note: Notecard probe moves before board probe so that config (including `sd_cs_pin`) is available for Adalogger's `Probe()`. The Notecard only needs I2C, which is already up. If the Notecard isn't present, fall back to SD card config — but this creates a chicken-and-egg situation for `sd_cs_pin` on first boot without a Notecard. Resolution: use the default CS pin (10) when no config is available. The user sets `sd_cs_pin` in Notehub if they've wired it differently; on next boot the Notecard delivers the override.

#### 2. Notecard I2C driver — `internal/notecard/notecard.go`

Minimal I2C JSON request/response wrapper for the Blues Notecard (address `0x17`). Build request JSON with `append` (no `encoding/json`). Parse responses with byte scanning.

Key operations:
- `Probe(bus drivers.I2C) (*Notecard, error)` — send `card.version`, verify response
- `EnvGet(name string) (string, error)` — pull a single env var
- `HubSet(productUID, sn string) error` — configure Notecard identity
- `NoteAdd(file string, body []byte) error` — queue a Note for sync
- `HubSync() error` — force sync on flush

#### 3. Device ID in flash — `internal/mcu/` interface + `internal/mcu/samd21/nvm.go`

Add `DeviceID() (string, error)` and `SetDeviceID(id string) error` to `hal.MCU`. SAMD21 implementation stores a short string in a reserved NVM flash row (64 bytes). On first boot, generate `"gloom-"` + 4 random hex chars and write to flash. Add `DeviceID` field to `config.Config`.

#### 4. Config cascade — Notecard → SD card → default

Config is managed entirely online via Blues Notehub environment variables (project → fleet → device hierarchy). The Notecard caches env vars locally on the module, so reads succeed even when cellular is down. On each boot:

1. Read device ID from flash (generate + write if empty)
2. If Notecard available: `env.get` config keys → parse into Config; on success, cache to SD card `config.ini` as backup
3. Else if SD card available: read `config.ini` → parse (stale but functional)
4. Else: `config.Default()`

The SD card is a secondary black-box backup — not the source of truth. A researcher changes config in the Notehub dashboard; the device picks it up on next boot via the Notecard. The SD card copy is there so the device can still boot with last-known-good settings if the Notecard is removed or fails.

#### 5. Notecard data/log sink — `internal/sink/notecard/`

Implements `sensor.Recorder` and `log.Sink`. Queue measurements into `data.qo` via `note.add`; queue error-level logs into `logs.qo`. Optionally `hub.sync` on `Flush()`.

### nRF52840 as second MCU target

Add the Nordic nRF52840 (Adafruit Feather nRF52840 Express) as the second supported MCU. 256KB RAM, Cortex-M4F, BLE 5.0, excellent TinyGo support, Feather form factor (FeatherWings plug in directly). Very low power: System OFF ~0.3µA, System ON with RTC ~1.5µA.

#### 1. MCU implementation — `internal/mcu/nrf52/nrf52.go`

Implement `hal.MCU` for the nRF52840:

- **`Standby()`** — use System ON sleep with `__WFE`. The nRF52 sleep model differs from SAMD21: no explicit SLEEPDEEP bit, instead the CPU sleeps automatically when all events are handled. Wake via RTC COMPARE event or GPIO SENSE.
- **`PrepareStandby()`** — configure GPIOTE for wake-on-pin (alarm interrupt from RTC FeatherWing). Enable RTC2 or similar peripheral as wake source.
- **`EnableWake(pin)` / `DisableWake(pin)`** — configure GPIO SENSE (high-to-low or low-to-high) on the given pin for wake from System ON sleep.
- **`EnableWatchdog()` / `PetWatchdog()`** — nRF52 WDT is simpler: write reload register, enable. WDT runs during sleep (unlike SAMD21) so timeout must be long enough to cover sleep duration, or pause/reload before sleep.
- **`DeviceID()` / `SetDeviceID()`** — use UICR (User Information Configuration Registers) for persistent storage, or the FICR device address registers for a factory-unique ID.

#### 2. Build-tagged entry point — `cmd/gloom/main_feather_nrf52840.go`

```go
//go:build feather_nrf52840
```

Provides `initMCU() hal.MCU` and `debugWriter() *machine.UART`. The nRF52840 Feather exposes UART on D0/D1 via the standard `machine.UART0`, so no manual SERCOM configuration is needed (unlike SAMD21).

#### 3. Sleep considerations

The nRF52840 WDT keeps running during sleep — it cannot be halted. Options:
- Set a long WDT timeout (e.g. 60s) and accept the coarser granularity.
- Reload the WDT from the RTC interrupt handler before re-entering sleep for multi-minute intervals.
- Use the nRF52 RTC peripheral (not the external DS3231/PCF8523) as an additional or fallback timer if no FeatherWing RTC is present.

#### 4. BLE configuration (future)

The nRF52840's built-in BLE could enable field configuration without physical SD card access or Notecard. A BLE characteristic could expose config key-value pairs, replacing or supplementing the Notecard env var flow. This is a stretch goal — the existing I2C FeatherWing flow works first.

### RP2040 as stable-power MCU target

Add the RP2040 (Adafruit Feather RP2040, ~$12) as a budget MCU target for deployments with a stable power source (mains, solar, large battery). 264KB RAM, dual Cortex-M0+, excellent TinyGo support, widely available. The RP2040 lacks a true ultra-low-power sleep mode — its dormant mode draws ~0.18mA at the chip level but ~2-4mA board-level due to the voltage regulator and peripherals. With a 10,000mAh battery and a 5-minute sample interval, expect roughly 4 months of runtime. This makes it unsuitable for long-term battery-only deployments (where SAMD21 or nRF52840 last years) but perfectly viable when connected to mains power, solar with a charge controller, or a large battery bank that is periodically serviced.

#### 1. MCU implementation — `internal/mcu/rp2040/rp2040.go`

Implement `hal.MCU` for the RP2040:

- **`Standby()`** — use dormant mode with GPIO wake. The RP2040 can be woken from dormant by an edge on a GPIO pin (e.g. from an external RTC alarm). Alternatively, use `__WFE` sleep (lighter, ~1-3mA board-level) if dormant mode proves unreliable under TinyGo.
- **`EnableWatchdog()` / `PetWatchdog()`** — RP2040 has a hardware watchdog with configurable timeout. Straightforward register access.
- **`DeviceID()` / `SetDeviceID()`** — use the RP2040's unique board ID (from flash) for read-only identity, or reserve a flash sector for writable persistent storage.

#### 2. Build-tagged entry point — `cmd/gloom/main_feather_rp2040.go`

```go
//go:build feather_rp2040
```

The Feather RP2040 exposes UART on D0/D1 via `machine.UART0`. No manual peripheral configuration needed.

#### 3. Sleep considerations

The RP2040 watchdog runs during dormant mode. Unlike the nRF52, the RP2040 watchdog has a maximum timeout of ~8.3 seconds, so it must be paused or the timeout extended before entering dormant sleep. The TinyGo `machine` package may handle some of this.

Without an external RTC FeatherWing, the RP2040 has no way to keep time during dormant mode (its internal timer halts). The `hal.Fallback` platform with `time.Sleep`-based scheduling may be the simplest approach for RP2040 deployments without an RTC, accepting the higher power draw of non-dormant sleep.
