Remove cfg.HeartbeatLedPin checks from manager — hardware topology
does not belong in the manager.

manager.step() currently gates m.blinkLED() on both
`cfg.HeartbeatLedPin != hal.NoPin` and `m.blinkLED != nil`.
main.go already only calls SetBlinkLED when the pin is valid, so
`m.blinkLED != nil` is the complete guard. The pin check is redundant
and is a hardware concern that has leaked in.

Changes:
- internal/manager/manager.go: replace all
  `m.cfg.HeartbeatLedPin != hal.NoPin && m.blinkLED != nil` guards
  with just `m.blinkLED != nil`
- internal/manager/manager_test.go: update any tests that rely on the
  pin guard

Run `make test` to verify.
