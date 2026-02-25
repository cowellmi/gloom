Fix double blink on heartbeat in manager.

manager/manager.go step() calls m.blinkLED() twice when hbFired:
once inside the `if hbFired` log-building block (~line 121), and again
in a standalone guard below. Remove the first call. Keep only the
standalone call after logging.

Files: internal/manager/manager.go, internal/manager/manager_test.go
