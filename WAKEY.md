# Wake/Sleep Design

## Overview

The device spends most of its time in deep sleep. It wakes in response to
one of three event types, determines why it woke, and takes the minimum
action necessary before going back to sleep.

## Wake Reasons

| Reason      | Trigger                        | Action                                      |
|-------------|--------------------------------|---------------------------------------------|
| Sample      | RTC Alarm 1 or sensor interrupt | Power on rails, init sensors, measure, transmit |
| Heartbeat   | RTC Alarm 2                    | Transmit a short keep-alive message (rails stay off) |

`WakeReason` is defined in `internal/hardware/platform.go` as `WakeSample`
and `WakeHeartbeat`.

## Platform Interface

```go
Sleep(sample, heartbeat time.Duration) (WakeReason, error)
```

- A zero duration disables that alarm.
- The device wakes on whichever event fires first.
- The returned `WakeReason` tells the main loop which path to take.

## RTC Alarms

The DS3231 has two hardware alarm slots. They map directly:

- **Alarm 1** -> sample interval (the main measurement cycle)
- **Alarm 2** -> heartbeat interval (keep-alive only)

Both alarms share the DS3231 INT pin. On wake the driver reads the status
register to determine which alarm fired.

## Sensor Interrupts

Some sensors can fire an interrupt to signal a time-critical event (e.g. a
tipping bucket gauge tips). These interrupts are wired to GPIO pins on the
board and configured as external wake sources at the platform level during
init, not by the sensor drivers themselves -- pin assignments are a
board-level concern.

Any sensor interrupt is treated the same as a sample wake: power on the
rails, measure everything, transmit. There is no need to distinguish which
sensor fired the interrupt.

## Main Loop

```
sleep -> wake -> branch on reason -> act -> repeat
```

- `WakeSample`: full measurement cycle
- `WakeHeartbeat`: transmit heartbeat, skip sensors entirely
