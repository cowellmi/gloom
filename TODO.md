# TODO

## `detachUSB` / `BeginSerial` symmetry

In `internal/mcu/samd21/samd21.go`, `detachUSB` guards on USB being enabled but `BeginSerial` does not. They should both check state before acting. Also `detachUSB` is unexported while `BeginSerial`/`EndSerial` are exported -- make the visibility consistent.

## GCLK6 RUNSTDBY race in `PrepareStandby`

In `internal/mcu/samd21/samd21.go`, GCLK `GENCTRL` is a multiplexed register keyed by the ID field. The initial `Set()` writes ID=6 correctly, but the subsequent `SetBits(RUNSTDBY)` is a read-modify-write. If the read-back returns a stale ID (due to sync latency or peripheral muxing), the RUNSTDBY bit could land on the wrong generator. Fold RUNSTDBY into the single `Set()` call to eliminate the race and the extra sync loop:

```go
sam.GCLK.GENCTRL.Set(
    sam.GCLK_GENCTRL_GENEN |
        sam.GCLK_GENCTRL_RUNSTDBY |
        sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
        6<<sam.GCLK_GENCTRL_ID_Pos,
)
```

## Blues Notecard sink for cloud connectivity

Add a Notecard sink at `sink/notecard/notecard.go` implementing both `sensor.Recorder` and `log.Sink`. The Notecard is a cellular module that communicates over I2C (address `0x17`) using JSON commands and provides store-and-forward sync to Notehub.

**Measurements:** Use `note.add` to queue readings into a `data.qo` Notefile. Build the JSON payload with `append` (maybe `orsinium-labs/jsony`?) to stay within 32KB RAM. Each Note body carries the device name, label, value, and unit. The Notecard syncs to Notehub on its own schedule -- the MCU never blocks on network.

**Logs:** Queue error-level (or higher) log entries into a `logs.qo` Notefile via `note.add`. Consider using `"sync":true` for critical errors to trigger immediate upload.

**Flush:** Optionally send `hub.sync` on `Flush()` to force a sync before sleep, or let the Notecard manage its own sync cadence to save power.

**Wiring:** Register the Notecard sink in `targets/hypnos-m0/main.go` alongside the serial and file sinks. I2C is already configured (`machine.I2C0`). The Notecard manages its own modem sleep independently, so the existing sleep/wake cycle does not change.

**Routing:** From Notehub, data can be routed to MQTT brokers, HTTP endpoints, AWS IoT, or any other backend. This removes the need to run an MQTT client on the MCU.
