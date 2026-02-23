# Research config-defined UART

**severity:** low

Investigate whether UART pin assignments can be made configurable at runtime (e.g. via INI or Notecard env vars) rather than compile-time board files. On the SAMD21, UART0 is bound to SERCOM0 with fixed PAD muxing, so arbitrary pin reassignment isn't possible without changing the SERCOM. Explore whether TinyGo exposes enough SERCOM flexibility to support this, and whether other targets (nRF52, RP2040) have more runtime flexibility. Currently all UART pins are set in `cmd/gloom/main_feather-m0.go` at compile time.
