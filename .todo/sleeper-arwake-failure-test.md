# Testing: No test for `deepSleep → idleSleep` fallback when `ArmWake` fails

**severity:** low

## Location

`internal/sleeper/sleeper_test.go`

## Problem

`sleeper_test.go` covers:
- Deep sleep when conditions are met (`TestSleep_DeepSleepSequence`)
- Idle sleep when remaining time is too short (`TestSleep_InsufficientRemaining_NoDeepSleep`)
- Rail sequencing for the happy path (`TestSleep_RailSequencing`)

But there is no test for the case where `deepSleep` is attempted, fails at `ArmWake`, and the code falls back to `idleSleep`. This path exercises:
- The rollback loop in `deepSleep` (disarming previously-armed pins)
- The rail state handed to `idleSleep` (rails are `RailsOff` at failure time)
- Whether `Sleep` ultimately restores `RailsCore` after the fallback

The related bug (`idlesleep-infinite-loop-on-failed-deepsleep.md`) is only reachable through this path.

## Test to add

Add a `mockMCU` variant (or extend the existing one) that returns an error from `ArmWake` on a specified call index. Then:

```go
func TestSleep_DeepSleepFallbackToIdle(t *testing.T) {
    mcu := &mockMCUWithArmError{failAt: 1}  // fail on second ArmWake call
    rtc := &mockRTC{times: []time.Time{T, T.Add(11 * time.Second)}, pin: 12}
    rails := &mockRails{}
    s := New(mcu, rtc, rails)
    s.AddWakePin(7)

    _, err := s.Sleep(T.Add(10 * time.Second))
    if err != nil {
        t.Fatalf("Sleep() returned error: %v", err)
    }

    // Standby should NOT have been called (deep sleep aborted).
    if callIndex(mcu.calls, "Standby") >= 0 {
        t.Error("Standby should not be called after ArmWake failure")
    }

    // Rail should be restored to RailsCore after the fallback.
    last := rails.powerCalls[len(rails.powerCalls)-1]
    if last != hal.RailsCore {
        t.Errorf("last Power call = %v, want RailsCore", last)
    }
}
```

Also add a test asserting that the first pin is properly disarmed during rollback (verifying the off-by-one fix in the rollback loop, once that is applied).
