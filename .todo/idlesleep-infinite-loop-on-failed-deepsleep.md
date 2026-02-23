# Bug: `idleSleep` can hang indefinitely when `deepSleep` fails after cutting rails

**severity:** bug (field-observable hang; requires obscure error path)

## Location

`internal/sleeper/sleeper.go` — interaction between `deepSleep` and `idleSleep`

## Sequence of events

1. `deepSleep` is called. It cuts rails to `RailsOff` (line 161), then calls `ArmWake` for each wake pin.
2. `ArmWake` fails on one of the pins. `deepSleep` rolls back and returns an error — but **rails are still `RailsOff`**.
3. `Sleep` catches the error and calls `idleSleep(target)` as a fallback.
4. In `idleSleep`, `readTime()` is called to check remaining duration. If the RTC depends on a core rail for I2C bus power (pull-ups), the read fails and the error is silently ignored, leaving `now` as `time.Time{}` (year 0001).
5. Because `target.Sub(now)` is enormous, `idleSleep` does restore `RailsCore` in this case — **so the hang is avoided if remaining time > `minDeepSleep`**.
6. However, if remaining time is small (< `minDeepSleep`), the condition is false, rails are **not** restored within `idleSleep`, and the busy-wait loop starts:

```go
for now.Before(target) {      // time.Time{}.Before(valid target) == always true
    s.mcu.PetWatchdog()
    remaining := min(target.Sub(now), tick)
    wait.For(remaining)
    now, _ = s.readTime()     // fails silently; now stays time.Time{}
}
```

The loop pets the watchdog, so the WDT never fires. The loop never exits. The device hangs until power-cycled.

After `idleSleep` returns, `Sleep` would restore `RailsCore` (line 133) — but execution never gets there.

## Conditions required

- `deepSleep` fails at `ArmWake` after rails are already cut (requires an EIC configuration failure — rare but possible on first boot or after reset)
- Remaining time < `minDeepSleep` (2 seconds) when the fallback occurs
- RTC I2C bus depends on a core rail for pull-up power

## Fix options

**Option A** (minimal): Restore `RailsCore` at the top of `idleSleep` unconditionally, before the first `readTime` call. The delay is negligible for a fallback path.

```go
func (s *Device) idleSleep(target time.Time) {
    // Restore core rails in case deepSleep cut them before failing.
    if s.rails != nil {
        s.rails.Power(hal.RailsCore)
    }
    ...
}
```

**Option B**: Restore rails in `Sleep` immediately after `deepSleep` returns an error, before calling `idleSleep`:

```go
if err := s.deepSleep(target); err != nil {
    if s.rails != nil {
        s.rails.Power(hal.RailsCore)
    }
    s.idleSleep(target)
}
```

Option A is simpler and makes `idleSleep` self-contained.

## Related

See also `sleeper-skip-rtc-error-comment.md`, which covers a different but adjacent issue: the silently-ignored `readTime` error in `Sleep` at line 103 (pre-sleep guard).
