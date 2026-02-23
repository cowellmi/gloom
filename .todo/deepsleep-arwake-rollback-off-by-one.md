# Bug: `deepSleep` ArmWake rollback loop is off-by-one

**severity:** bug (low risk — harmless on SAMD21, but semantically wrong)

## Location

`internal/sleeper/sleeper.go:164-170`

```go
for i, pin := range s.wakePins {
    if err := s.mcu.ArmWake(pin); err != nil {
        for j := 0; j <= i; j++ {   // ← off-by-one
            s.mcu.DisarmWake(s.wakePins[j])
        }
        return err
    }
}
```

## Problem

When `ArmWake(s.wakePins[i])` fails, the rollback disarms pins `0..i` inclusive. But `s.wakePins[i]` was never successfully armed — only pins `0..i-1` were. The bound should be `j < i`.

On SAMD21, `DisarmWake` calls `p.SetInterrupt(0, nil)` and clears the EIC WAKEUP bit. Calling this on a pin that was never registered is harmless (clears an already-clear bit), so there is no observable failure. But the intent is wrong and it masks the actual error surface.

## Fix

```go
for j := 0; j < i; j++ {
    s.mcu.DisarmWake(s.wakePins[j])
}
```
