# RP2040 as stable-power MCU target

**severity:** feature

Add the RP2040 (Adafruit Feather RP2040, ~$12) as a budget MCU target for deployments with a stable power source (mains, solar, large battery). 264KB RAM, dual Cortex-M0+, excellent TinyGo support, widely available. The RP2040 lacks a true ultra-low-power sleep mode — its dormant mode draws ~0.18mA at the chip level but ~2-4mA board-level due to the voltage regulator and peripherals. With a 10,000mAh battery and a 5-minute sample interval, expect roughly 4 months of runtime. This makes it unsuitable for long-term battery-only deployments (where SAMD21 or nRF52840 last years) but perfectly viable when connected to mains power, solar with a charge controller, or a large battery bank that is periodically serviced.

## 1. MCU implementation — `internal/mcu/rp2040/rp2040.go`

Implement `hal.MCU` for the RP2040:

- **`Standby()`** — use dormant mode with GPIO wake. The RP2040 can be woken from dormant by an edge on a GPIO pin (e.g. from an external RTC alarm). Alternatively, use `__WFE` sleep (lighter, ~1-3mA board-level) if dormant mode proves unreliable under TinyGo.
- **`EnableWatchdog()` / `PetWatchdog()`** — RP2040 has a hardware watchdog with configurable timeout. Straightforward register access.
- **`DeviceID()` / `SetDeviceID()`** — use the RP2040's unique board ID (from flash) for read-only identity, or reserve a flash sector for writable persistent storage.

## 2. Build-tagged entry point — `cmd/gloom/main_feather_rp2040.go`

```go
//go:build feather_rp2040
```

The Feather RP2040 exposes UART on D0/D1 via `machine.UART0`. No manual peripheral configuration needed.

## 3. Sleep considerations

The RP2040 watchdog runs during dormant mode. Unlike the nRF52, the RP2040 watchdog has a maximum timeout of ~8.3 seconds, so it must be paused or the timeout extended before entering dormant sleep. The TinyGo `machine` package may handle some of this.

Without an external RTC FeatherWing, the RP2040 has no way to keep time during dormant mode (its internal timer halts). The `hal.Fallback` platform with `time.Sleep`-based scheduling may be the simplest approach for RP2040 deployments without an RTC, accepting the higher power draw of non-dormant sleep.
