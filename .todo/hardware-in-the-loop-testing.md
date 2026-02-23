# Hardware-in-the-loop (HIL) testing

**severity:** feature

Set up a physical Feather M0 + Hypnos board connected to a CI runner for automated integration testing against real hardware. The target-agnostic packages are already covered by `go test`, but the hardware-dependent code (`targets/samd21/`, `drivers/ds3231/`, `power/`) has no automated test path today.

## Approach

1. Dedicate a Feather M0 + Hypnos wired to a host machine (Raspberry Pi or spare Linux box) via both USB (for flashing via bossac) and a USB-to-serial adapter on D0/D1 (for UART capture).
2. CI job runs `make flash`, then opens the serial port and asserts on expected UART output: successful boot, RTC probe, SD probe, config load, sensor read, sleep entry.
3. Use a simple expect-style harness (shell script or small Go program) that reads lines from the serial port with a timeout. Pass/fail is determined by whether the expected log lines appear in order within the timeout window.
4. A watchdog reset or panic (visible on USB CDC) is an automatic failure.

## What it covers

- Full boot ceremony: rail power cycle, I2C bus init, RTC and SD probe.
- Config load from SD card (place a known `config.ini` on the card before the test).
- At least one successful sleep/wake cycle with DS3231 alarm fire.
- Sensor sampling (VBAT at minimum) and serial recorder output.
- Watchdog petting (no reset within the test window).

## What it doesn't cover

- Multi-hour battery life or deep-sleep current draw (needs a power profiler, not CI).
- Edge cases like SD card corruption or I2C bus lockup (would need fault injection hardware).
