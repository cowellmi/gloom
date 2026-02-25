# nRF52840 as second MCU target

**severity:** feature

Add the Nordic nRF52840 (Adafruit Feather nRF52840 Express) as the second supported MCU. 256KB RAM, Cortex-M4F, BLE 5.0, excellent TinyGo support, Feather form factor (FeatherWings plug in directly). Very low power: System OFF ~0.3µA, System ON with RTC ~1.5µA.

## 1. MCU implementation — `internal/mcu/nrf52/nrf52.go`

Implement `hal.MCU` for the nRF52840:

- **`Standby()`** — use System ON sleep with `__WFE`. The nRF52 sleep model differs from SAMD21: no explicit SLEEPDEEP bit, instead the CPU sleeps automatically when all events are handled. Wake via RTC COMPARE event or GPIO SENSE.
- **`PrepareStandby()`** — configure GPIOTE for wake-on-pin (alarm interrupt from RTC FeatherWing). Enable RTC2 or similar peripheral as wake source.
- **`EnableWake(pin)` / `DisableWake(pin)`** — configure GPIO SENSE (high-to-low or low-to-high) on the given pin for wake from System ON sleep.
- **`EnableWatchdog()` / `PetWatchdog()`** — nRF52 WDT is simpler: write reload register, enable. WDT runs during sleep (unlike SAMD21) so timeout must be long enough to cover sleep duration, or pause/reload before sleep.
- **`DeviceID()` / `SetDeviceID()`** — use UICR (User Information Configuration Registers) for persistent storage, or the FICR device address registers for a factory-unique ID.

## 2. Build-tagged entry point — `cmd/gloom/main_feather_nrf52840.go`

```go
//go:build feather_nrf52840
```

Provides `initBoard() Board` with MCU, peripherals, and pin assignments. The nRF52840 Feather exposes UART on D0/D1 via `machine.UART0`.

## 3. Sleep considerations

The nRF52840 WDT keeps running during sleep — it cannot be halted. Options:
- Set a long WDT timeout (e.g. 60s) and accept the coarser granularity.
- Reload the WDT from the RTC interrupt handler before re-entering sleep for multi-minute intervals.
- Use the nRF52 RTC peripheral (not the external DS3231/PCF8523) as an additional or fallback timer if no FeatherWing RTC is present.

## 4. BLE configuration (future)

The nRF52840's built-in BLE could enable field configuration without physical SD card access or Notecard. A BLE characteristic could expose config key-value pairs, replacing or supplementing the Notecard env var flow. This is a stretch goal — the existing I2C FeatherWing flow works first.
