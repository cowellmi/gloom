# Bug: `board.ADCPin != 0` uses wrong sentinel for "not configured"

**severity:** bug (benign on Feather M0, but wrong semantics)

## Location

`cmd/gloom/main.go:119`

```go
if board.ADCPin != 0 {
    sensorRegistry["vbat"] = func() sensor.Sensor {
        return vbat.NewDevice(board.ADCPin)
    }
}
```

## Problem

GPIO pin 0 (PA00 on SAMD21) is a valid pin number. The project's sentinel for "no pin configured" is `hal.NoPin` (0xFF), used consistently everywhere else — `config.Group.ExternalIntPin`, `sleeper.AddWakePin` deduplication, etc. Using `!= 0` here is semantically wrong: a board file that legitimately assigns ADC to pin 0 would silently disable the vbat sensor.

On the Feather M0, the battery ADC is on D9 (PA05 = pin 5), so `!= 0` happens to work. But if a future board file uses `board.ADCPin = hal.NoPin` to signal "no ADC", the comparison would incorrectly register the vbat sensor with pin 255 (0xFF) passed to `vbat.NewDevice`.

## Fix

```go
if board.ADCPin != hal.NoPin {
```

Also update `board.go`'s `Board` struct comment and any board files that leave `ADCPin` at its zero value to explicitly set `hal.NoPin` instead.
