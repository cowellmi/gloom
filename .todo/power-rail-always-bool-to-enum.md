# Refactor: replace `always bool` in `power.NewRail` with a named type

**severity:** low (ergonomics / readability)

## Current API

```go
// internal/power/power.go
func NewRail(pin hal.Pin, polarity Polarity, always bool, delay time.Duration) Rail

// cmd/gloom/power_hypnos.go
power.NewRail(hal.Pin(machine.D5), power.ActiveLow, true,  0)                   // core
power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, false, 250*time.Millisecond) // sensors
```

`true` and `false` at the call site are opaque — a reader has to look up the signature to know which is which.

## Goal

Replace `always bool` with a named type that expresses what power state activates each rail, without requiring a reader to remember which bool value means what.

The naming is unsettled. Options in rough order of preference:

---

### Option A — Use `hal.RailState` as the activation threshold (no new type)

Each rail declares the minimum `RailState` at which it should be enabled:

```go
func NewRail(pin hal.Pin, polarity Polarity, activateAt hal.RailState, delay time.Duration) Rail

power.NewRail(hal.Pin(machine.D5), power.ActiveLow,  hal.RailsCore, 0)
power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, hal.RailsFull, 250*time.Millisecond)
```

`Power(state)` enables a rail when `state >= rail.activateAt`. Reuses the existing enum, no new type needed, and the semantics are self-documenting: "enable this rail when the system reaches at least Core / Full power."

---

### Option B — New `RailGroup` type with domain-specific names

```go
type RailGroup uint8

const (
    RailGroupCore   RailGroup = iota // infrastructure: always on after wake
    RailGroupSensor                  // on-demand: only when sensors are active
)
```

Call site:
```go
power.NewRail(hal.Pin(machine.D5), power.ActiveLow,  power.RailGroupCore,   0)
power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, power.RailGroupSensor, 250*time.Millisecond)
```

Descriptive, but introduces a parallel vocabulary alongside `hal.RailsCore`/`hal.RailsFull`. May cause confusion about the relationship between `RailGroup` and `RailState`.

---

### Option C — `RailRole` with names that avoid the Core/Full/Always collision

```go
type RailRole uint8

const (
    Infrastructure RailRole = iota  // up whenever the board is awake
    OnDemand                        // up only when sensors are active
)
```

Call site:
```go
power.NewRail(hal.Pin(machine.D5), power.ActiveLow,  power.Infrastructure, 0)
power.NewRail(hal.Pin(machine.D6), power.ActiveHigh, power.OnDemand,       250*time.Millisecond)
```

More descriptive of *purpose* rather than *threshold*, but longer and doesn't generalize cleanly if a third rail category is ever needed.

---

## Recommendation (unresolved)

Option A (threshold = `hal.RailState`) is the most compositional: it's extensible (a third rail activated at `RailsOff` would just work), reuses an existing type, and makes the controller logic trivial (`enable if state >= activateAt`). The downside is that call sites must import `hal`, which `power.go` callers already do.

Option B is a reasonable middle ground if you want to keep the power package vocabulary self-contained.

The naming question (`Core`, `Sensor`, `Infrastructure`, `OnDemand`, etc.) is the main open decision.

## Files to change

- `internal/power/power.go` — `Rail.always bool` → `Rail.activateAt <chosen type>`, update `Power()` logic
- `cmd/gloom/power_hypnos.go` — update `NewRail` call sites
- `cmd/gloom/power_no_hypnos.go` — if it calls `NewRail`
- Any future board power files
