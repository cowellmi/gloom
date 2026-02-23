# Blues Notecard integration (config source + data sink)

**severity:** feature

Use the Notecard as the primary config source and a data/log sink. The Notecard communicates over I2C (`0x17`) using JSON commands and provides store-and-forward sync to Notehub. SD card becomes a local black-box backup rather than the single source of truth for config.

## 1. Notecard I2C driver — `internal/notecard/notecard.go`

Low-level I2C JSON request/response wrapper. Build request JSON with `append` (no `encoding/json`). Parse responses with minimal scanning to stay within 32KB RAM. Shared by both the config source and the data sink.

## 2. Device ID persistence — `internal/targets/samd21/nvm.go`

Store a short device ID string in a reserved SAMD21 flash row (64 bytes) via the NVM controller. Add `ReadDeviceID() (string, error)` and `WriteDeviceID(id string) error` to the MCU. Flash write cycles (~25K) are fine since the ID rarely changes. Add a `DeviceStore` interface so this stays target-agnostic.

## 3. Config from Notecard environment variables

Notehub environment variables are hierarchical (project → fleet → device). Set config keys (`sample_interval`, `sensors`, etc.) from the Notehub dashboard; devices pull them via `env.get`. The Notecard caches env vars locally, so reads succeed even when cellular is down.

## 4. Boot flow in `main.go`

1. Read device ID from flash. If empty, generate a random one (`"gloom-"` + 4 hex chars) and write it to flash.
2. Add `device_id` to `Config`. Use it as the Notecard's device identity and in CSV/log output.
3. Try `env.get` from the Notecard for config values. On success, cache to SD card `config.ini` as backup.
4. If Notecard unavailable, fall back to SD card `config.ini`.
5. If SD card also unavailable, use `config.Default()`.

This means a freshly flashed device with no SD card still boots, self-identifies, and becomes configurable from the cloud once the Notecard connects.

Once Blues config is loaded before SD probing, `sd_cs_pins` can be added back to `config.ini` so the Notecard can override board-default CS pins. Currently `SDCSPins` is only set by the board file (`initBoard()`) on the `Board` struct because config lives on the SD card (chicken-and-egg).

## 5. Data sink — `internal/sink/notecard/notecard.go`

Implement `sensor.Recorder` and `log.Sink`. Queue measurements into a `data.qo` Notefile via `note.add`; queue error-level logs into `logs.qo`. Let the Notecard manage its own sync cadence, or optionally `hub.sync` on `Flush()`. From Notehub, data routes to MQTT, HTTP, AWS IoT, etc.

## 6. SD card role change

SD card shifts from config authority to local backup: cached config, data logging, log files. If the card is missing or corrupt the device still runs. Existing file sink and retention logic stay unchanged.

## 7. Update config on env var updates from Blues Notehub

When we receive a message from Blues Notecard that there have been env vars updates for this device from Notehub, update the manager.cfg with the new values (or just do a hard reset?).
