  Architecture

  `gloom` is a low-power environmental sensor station targeting the Feather M0 + Hypnos shield. The design follows a simple sleep/wake loop:

  sleep -> wake -> branch on reason -> act -> repeat


  Key layers:

  • `Manager` (manager.go) -- the core orchestrator. Initializes hardware, parses config, runs the main loop. Each iteration sleeps, wakes, then either
    runs a full sample() cycle or a heartbeat() keep-alive.
  • `hardware.Platform` (platform.go) -- abstraction over the board. Two implementations:
    • `hypnos.Board` -- the real deal: DS3231 RTC over I2C, two MOSFET-controlled power rails (3.3V active-low on D5, 5V active-high on D6), SD card
      (stubbed).
    • `fallback.Platform` -- graceful degradation when Hypnos isn't detected; uses time.Sleep and time.Now() instead of RTC/deep sleep.
  • `sensors.Device` -- pluggable sensor interface (Init, Name, Measure). Sensors are registered by string ID in registry.go and selected via config.txt on
    the SD card. Only a fake sensor exists so far.
  • `Config` -- parsed from a simple key=value file (config.txt) read from SD. Falls back to DefaultConfig() (5s sample, serial on, debug logging, fake
    sensor).
  • `log.Logger` -- lightweight serial logger with RTC-sourced timestamps and level filtering. Uses bare print/println to stay allocation-friendly.


  Notable design choices:

  1. Deferred error reporting -- NewManager accumulates init errors into initErrs since the logger isn't ready until serial is configured. They get flushed
      once logging is live.
  2. I2C bus retry on wake -- waitForRTC retries SetRunning up to 3 times with 500ms delays, accounting for pull-up/oscillator stabilization after power
     rails come back.
  3. Rails gated by wake reason -- on heartbeat wakes, sensor power stays off entirely (no reason to power the rails just to send a keep-alive).
  4. Probe-based platform detection -- if the RTC doesn't respond on I2C, it falls back gracefully rather than panicking.


  Things still stubbed or TODO:

  • SD card -- ReadFile returns an error; config always falls back to defaults on real hardware.
  • Deep sleep -- Sleep() in Hypnos currently just calls time.Sleep() instead of using RTC alarm interrupts + ARM deep sleep (SAMD21 standby mode).
  • Heartbeat transmission -- heartbeat() is a no-op placeholder.
  • Sensor interrupt wakes -- documented in WAKEY.md but not yet wired up (GPIO external wake sources).
  • No real sensors registered -- only fake is in the registry.
