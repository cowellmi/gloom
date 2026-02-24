# Blues Notecard integration (config source + data sink)

**severity:** feature

Use the Notecard as the primary config source and a data/log sink. The Notecard communicates over I2C (`0x17`) using JSON commands and provides store-and-forward sync to Notehub. SD card becomes a local black-box backup rather than the single source of truth for config.

## Config design

Config uses a flat key=value format identical for both `config.ini` and Notehub environment variables. The 10 keys are:

```
log_sinks, data_sinks, led_pin        # Device
interval, sensors, ext_pin            # Sample
heartbeat, host, payload, blink_led   # Heartbeat
```

Two first-class concepts replace the old named-group system:
- **sample** — periodic or interrupt-driven sensor measurement
- **heartbeat** — optional keep-alive / payload delivery (disabled when `heartbeat = 0`)

`env.template` (hardcoded in firmware, sent every boot) registers all 10 keys with Notehub so they appear in the UI. Sending it every boot is idempotent and LoRa-safe.

## Boot flow (`main.go`)

1. `cfg = config.Default()` — sensible defaults (5s sample, no heartbeat)
2. `nc, _ = tinynote.OpenI2C(...)` — always succeeds
3. `card.version` request → if responds: `notecardPresent = true`, set `cfg.Device.ID`
4. If notecardPresent: send `env.template` (hardcoded body), then `env.get` → iterate body, call `config.ParseKey(&cfg, k, v)` for each key
5. If !notecardPresent && SD card present: read `CONFIG.INI` → `config.Parse(raw, &cfg)`
6. Resolve `cfg.Sample` → `groups[0]`, `cfg.Heartbeat` (if Interval > 0) → `groups[1]`

Since Notecard is probed before SD config loading, `sd_cs_pins` can be added to the 10-key schema in a future iteration, fixing the chicken-and-egg with SD CS pin config.

## Remaining work

### 1. Data sink — `internal/sink/notecard/notecard.go`

Implement `sensor.Recorder` and `log.Sink`. Queue measurements into a `data.qo` Notefile via `note.add`; queue error-level logs into `logs.qo`. Let the Notecard manage its own sync cadence, or optionally `hub.sync` on `Flush()`. From Notehub, data routes to MQTT, HTTP, AWS IoT, etc.

Add `notecard` as a valid sink name in `config.knownSinks` once implemented.

### 2. SD card as fallback backup

When Notecard is present, periodically write current config to `CONFIG.INI` on SD card so the device can fall back to last-known-good config if the Notecard is removed.

### 3. Env var update notifications

When Notehub pushes env var updates, the Notecard sets a flag readable via `env.modified`. Poll this flag and re-run the `env.get` + `ParseKey` loop, then either update the manager in place or trigger a soft reset.
