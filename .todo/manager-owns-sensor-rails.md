PowerOnSensorRails() does not belong on the sleeper — move rails control
to the manager.

Currently sleeper.Device holds hal.Rails and exposes PowerOnSensorRails()
so the manager can elevate power before measuring sensors. The sleeper's
job is sleep/wake orchestration; powering on sensors for measurements is
a manager concern.

Changes:
- internal/sleeper/sleeper.go: remove PowerOnSensorRails() method
- internal/manager/manager.go:
  - remove PowerOnSensorRails() from the local `sleeper` interface
  - add `rails hal.Rails` field to Manager struct
  - add rails parameter to manager.New()
  - before measureSensors(), call rails.Power(hal.RailsFull) directly
    (nil-check rails, same as sleeper did)
- internal/manager/manager_test.go: update mockSystem (remove
  PowerOnSensorRails), add mockRails if needed, update New() calls
- cmd/gloom/main.go: pass rails to manager.New()

Run `make test` to verify.
