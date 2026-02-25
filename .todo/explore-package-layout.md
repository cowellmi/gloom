Exploration: review the internal package layout for hardware implementations.

hal/mcu.go comments reference a `targets/<chip>/` layout but the actual
path is `internal/mcu/samd21/`. There are also `internal/power/`,
`internal/led/`, `internal/rtc/ds3231/` — all are hardware implementations
but they sit in different top-level directories with no consistent convention.

Explore whether these should be consolidated under a single subtree
(e.g. `internal/drivers/<chip>/`, `internal/targets/`, or `internal/hw/`)
and propose a layout with a rationale. Do not make changes — write up a
recommendation.
