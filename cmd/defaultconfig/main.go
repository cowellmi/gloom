package main

import (
	"os"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
)

// Write default config to ./default.config.json
// Uses pin defaults for build tags: feather_m0 && !no_hypnos
func main() {
	cfg := config.Default(
		hal.Pin(17), // machine.LED
		hal.Pin(19), // machine.D12
		[]string{"vbat"},
		[]hal.Pin{
			hal.Pin(16), // machine.D11
			hal.Pin(18), // machine.D10
		})

	b, err := cfg.MarshalJSON()
	if err != nil {
		panic(err)
	}

	err = os.WriteFile("./default.config.json", b, 0644)
	if err != nil {
		panic(err)
	}
}
