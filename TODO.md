# TODO

Items are ordered by severity: critical (data loss / field failure) first, then high (correctness / memory), medium (conventions / maintainability), low (polish), and finally features.

---

## High

### `formatTimestamp` in `sink/file` heap-allocates every call

In `internal/sink/file/file.go`, `formatTimestamp` returns `string(buf[:])` which escapes the stack-allocated `[19]byte` to the heap on every call. This runs once per measurement per cycle. On 32KB RAM it adds up. Refactor to accept a `[]byte` parameter and append into the caller's scratch buffer, consistent with how the serial sink formats timestamps.

### `Measure()` allocates `[]Measurement` per call

In `internal/sensor/fake/fake.go` (and any real sensor), `Measure()` returns a freshly allocated `[]sensor.Measurement` slice every call. On 32KB RAM, consider pre-allocating a fixed-size array in the Device struct and returning a slice of it, or changing the interface to `Measure(buf []Measurement) ([]Measurement, error)`.

### `debug.Log` allocates on every call

In `internal/debug/debug.go`, `append([]byte(nil), msg...)` allocates a new `[]byte` on every call. If `debug.Log` is used in any hot path, this adds unnecessary GC pressure on 32KB RAM. Use a package-level `[256]byte` scratch buffer instead, or accept a `[]byte` parameter.

### `WakeExternal` is silently ignored in `step()`

In `internal/manager/manager.go`, the `step()` switch handles `WakeSample` and `WakeHeartbeat` but `WakeExternal` falls through with no log entry. For field debugging, add a `logger.Debug("external wake")` case.

### I2C bus recovery on boot

The I2C bus can latch into a stuck state if the MCU resets (watchdog, brownout, debug flash) while a slave device (e.g. DS3231) is mid-transaction. The slave holds SDA low waiting for clocks that never come, and the SAMD21 SERCOM's `Configure()` cannot clear this — it sees the bus as busy and all subsequent transactions timeout.

The standard fix is a bit-banged bus recovery before configuring the I2C peripheral: temporarily configure SDA/SCL as GPIO outputs, toggle SCL 9+ times (giving the slave a chance to release SDA on each clock), generate a STOP condition (SDA low→high while SCL is high), then switch back to I2C mode. This is what Linux's `i2c_recover_bus()` does.

Currently `waitForReady` in `internal/rtc/ds3231/ds3231.go` reconfigures the SERCOM between retries, which helps with soft bus errors but not a fully latched slave. A proper recovery would live on `hal.MCU` (e.g. `RecoverI2C(sda, scl machine.Pin)`) since it's chip-specific register access, and would be called once in `main.go` before `machine.I2C0.Configure`.

### SD card SPI state not reset before probe when rail controller loads late

The power controller (`power.NewController(rails...)`) performs an initial power cycle to discharge SD card capacitors and reset its SPI state machine. This is important after a watchdog reset where the SD card can be stuck mid-command. Power rails are now enabled before peripheral probing (via build-tagged `boardPower()`), so the power cycle happens early. If SD card probe failures are observed after watchdog resets, this is the first place to look.

---

## Medium

### `maybeRotate` discards `openForDate` error

In `internal/sink/file/file.go`, `maybeRotate` calls `s.openForDate(t)` but ignores the returned error. Per project conventions, this should be `_ = s.openForDate(t)` with a comment, or the error should be propagated so callers know rotation failed.

### Config boolean values not documented for non-programmers

In `internal/config/config.go`, boolean fields like `serial` and `enable_led` only accept the exact string `"true"` to enable. Any other value (including `"yes"`, `"1"`, `"TRUE"`) silently means false. For a framework targeting non-programmers, either document the accepted values clearly in a sample config file, or accept common truthy variants (`"true"`, `"yes"`, `"1"`, case-insensitive).

### Shared `buf` in Manager needs concurrency note

In `internal/manager/manager.go`, the `[recorderBufSize]byte` scratch buffer is shared across all recorders and `logMem`. Currently single-threaded so it works, but if goroutine-based sensor polling is ever added this becomes a data race. Add a comment documenting the single-goroutine assumption.

### Duplicate `appendLevel` function

Both `internal/sink/serial/serial.go` and `internal/sink/file/file.go` define identical `appendLevel` helper functions. Extract to a shared location (e.g. `internal/log/` or a small `internal/format/` package) to avoid drift.

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

### Adalogger FeatherWing as a `hal.Platform` implementation

Add the Adalogger FeatherWing (PCF8523 RTC + SD card) as a second board.

#### 1. PCF8523 RTC driver — `internal/boards/adalogger/rtc.go`

There is no PCF8523 driver in `tinygo.org/x/drivers`. Write a minimal driver covering only what the board needs:

- `ReadTime() (time.Time, error)` — read date/time registers (0x03–0x09, BCD-encoded).
- `SetTime(t time.Time) error` — write date/time registers.
- `SetCountdownTimer(d time.Duration) error` — configure Timer A or B as a countdown source. The countdown timer has selectable source clocks (4096 Hz, 64 Hz, 1 Hz, 1/60 Hz), so pick the coarsest clock that covers the requested duration. Assert INT on expiry.
- `ClearTimerInterrupt() error` — clear the timer flag so the INT pin releases.
- `Configure()` — initialize oscillator, disable unused features (CLKOUT, alarms if not used).

The countdown timer is a better fit than the PCF8523 alarm for `Sleep()` because the alarm only has minute-level granularity while the timer supports sub-second precision. The timer also maps directly to durations, which is what `Sleep()` receives.

#### 2. Board implementation — `internal/boards/adalogger/adalogger.go`

`Board` struct implementing `hal.Platform`:

```go
type Board struct {
    proc          hal.MCU
    rtc           *pcf8523.Device   // or local struct
    Card          *sdcard.Card
    rails         []RailConfig
    nextSample    time.Time
    nextHeartbeat time.Time
}
```

- **`Identifier()`** — returns `"Adalogger"` + MCU identifier.
- **`ReadTime()`** — delegates to PCF8523.
- **`Sleep()`** — compute the shortest deadline, set PCF8523 countdown timer, configure INT pin as wake source, enter SAMD21 STANDBY, clear timer flag on wake. No `powerOnDelay` subtraction needed when rails are absent.

SD card CS is pin 10 on the Adalogger FeatherWing.

The INT pin from the PCF8523 needs to be wired to a Feather GPIO for wake-from-standby. Accept this as a parameter to `Probe()` (the Adalogger FeatherWing routes INT to a header pad, not a fixed Feather pin).

#### 3. Optional MOSFET power rails — DONE

`config.RailConfig` and the generic `power.Controller` now handles arbitrary GPIO rails with configurable polarity and `WakeReason` bitmask. Each rail carries a `hal.WakeReason` (`WakeAlways` for core, `WakeSample` for sensors) so sensor rails stay off during heartbeat wakes. `power = hypnos` is a built-in preset; researchers can wire custom MOSFETs and set `power_rails = 5:low, 9:sample` in `config.ini`.

#### 4. `sensor.Sleeper` interface — `internal/sensor/sensor.go`

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

#### 5. Board file — `cmd/gloom/main_adalogger_m0.go`

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

**Step B — Adalogger board implementation** (`internal/boards/adalogger/`). PCF8523 RTC driver + `hal.Platform`. The Adalogger FeatherWing uses SD card CS on pin 10 by default, but the CS pin should be configurable — add `sd_cs_pin` to config so users can wire an SD card reader to any GPIO. This works because config lives in Notehub (Blues Notecard env vars) and is pushed to the device over the air. The Notecard caches env vars locally, so they're available even when cellular is offline. On boot, config syncs down to SD card `config.ini` as a backup, giving graceful degradation when neither Notecard nor connectivity is available.

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
