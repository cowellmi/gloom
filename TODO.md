`detachUSB` (`internal/mcu/samd21/samd21.go`) guards on USB being enabled; `BeginSerial` doesn't. Also `detachUSB` is unexported while `BeginSerial`/`EndSerial` are exported.


--


GCLK6 RUNSTDBY set via read-modify-write on a multiplexed register

In `PrepareStandby` (`internal/mcu/samd21/samd21.go`), GCLK `GENCTRL` is a multiplexed register keyed by the ID field. The initial `Set()` on line 67 writes ID=6 correctly. But line 76 uses `SetBits(RUNSTDBY)`, which is a read-modify-write. If the read-back returns a stale ID (due to sync latency or peripheral muxing), the RUNSTDBY bit could land on the wrong generator. Fold it into the single `Set()` call:

```go
sam.GCLK.GENCTRL.Set(
    sam.GCLK_GENCTRL_GENEN |
        sam.GCLK_GENCTRL_RUNSTDBY |
        sam.GCLK_GENCTRL_SRC_OSCULP32K<<sam.GCLK_GENCTRL_SRC_Pos |
        6<<sam.GCLK_GENCTRL_ID_Pos,
)
```

This eliminates the extra sync loop too.
