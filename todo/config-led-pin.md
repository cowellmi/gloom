# Feature: CONFIG.INI led_pin field with per-target default

## Background

`cmd/gloom/board.go` defines `Board.ConfigureLED(pin hal.Pin)` which reassigns the board LED
at runtime, but it is never called — there is no config field that maps to it, so the method is
dead code today.

## Desired behaviour

`CONFIG.INI` should support an optional `led_pin` key under `[device]`:

```ini
[device]
led_pin = 13   # optional; defaults to the target's machine.LED pin
```

If `led_pin` is absent the board uses its hardware default (e.g. `machine.LED` = pin 13 on
Feather M0). If present, `Board.ConfigureLED` is called after config is parsed.

## Implementation sketch

1. **`internal/config/config.go`** — add `LEDPin hal.Pin` to `Device` struct (default
   `hal.NoPin` meaning "use board default"). Parse `led_pin` in `parseDeviceKey`. Emit it in
   `Marshal` only when `!= hal.NoPin`.

2. **`cmd/gloom/main.go`** — after config is parsed and before any LED use, check
   `cfg.Device.LEDPin != hal.NoPin` and call `board.ConfigureLED(cfg.Device.LEDPin)`.

3. **`cmd/gloom/board_feather-m0.go`** — no change needed; `newLED(machine.LED)` in
   `initBoard` sets the default which `ConfigureLED` can override.

## Notes

- `hal.Pin(0xff)` (`hal.NoPin`) is the sentinel for "not configured" — consistent with how
  `ExternalIntPin` works.
- `ConfigureLED` already exists and is correct; this is purely a config wiring task.
- Add a test to `config_test.go` covering `led_pin` parse + marshal round-trip.
