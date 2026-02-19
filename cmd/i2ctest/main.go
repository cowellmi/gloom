// Command i2ctest verifies I2C bus recovery on Hypnos hardware.
//
// It hammers DS3231 ReadTime in a tight loop, then lets the watchdog
// fire mid-transaction. On the next boot, RecoverI2C must unstick the
// bus so the DS3231 probes cleanly.
//
// Flash and monitor:
//
//	tinygo flash -target=feather-m0 ./cmd/i2ctest
//	picocom -b 115200 /dev/ttyUSB0
//
// The test runs in two phases per boot cycle:
//
//	Phase 1 — 500 ReadTime calls with watchdog petting. Hit the
//	          reset button during this window to test external-reset
//	          recovery.
//	Phase 2 — ReadTime loop without petting the watchdog. The ~8s
//	          WDT timeout fires mid-I2C-transaction, resetting the
//	          MCU. The next boot tests bus recovery.
//
// A successful recovery after a WDT reset prints:
//
//	** RECOVERED: WDT reset + stuck bus -> DS3231 OK **
package main

import (
	"machine"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/drivers/ds3231"
	"github.com/cowellmi/gloom/internal/wait"
)

func main() {
	// Latch the 3.3V rail ON immediately, before UART or watchdog
	// init. After a WDT reset the Hypnos gate pullup starts
	// turning D5 off (rail drops). If the rail drops far enough
	// the DS3231 enters battery mode, its I2C state machine resets,
	// and SDA releases — hiding the stuck bus we want to test.
	// Driving D5 LOW here races the gate pullup to keep VCC up so
	// a mid-transaction DS3231 stays stuck.
	machine.D5.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.D5.Low() // active-low = ON

	proc := initMCU()

	debug.Log("")
	debug.Log("=== I2C RECOVERY TEST ===")
	debug.Log("reset: " + resetCause())

	// D5 was latched above; now turn on D6 (5V sensor rail) for
	// completeness and wait for the bus pullups to stabilise.
	machine.D6.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.D6.High() // active-high = ON
	wait.For(50 * time.Millisecond)
	debug.Log("rails: ok")
	proc.PetWatchdog()

	// Read SDA with pullups active. If the DS3231 is holding SDA
	// from a prior stuck transaction, it reads LOW here.
	sda := uint8(machine.SDA_PIN)
	scl := uint8(machine.SCL_PIN)
	sdaPin := machine.Pin(sda)
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	wait.For(10 * time.Microsecond)
	stuck := !sdaPin.Get()
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	if stuck {
		debug.Log("SDA: LOW (bus stuck)")
	} else {
		debug.Log("SDA: HIGH (idle)")
	}

	proc.RecoverI2C(sda, scl)
	debug.Log("recover: done")

	// Verify SDA released.
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	wait.For(10 * time.Microsecond)
	released := sdaPin.Get()
	sdaPin.Configure(machine.PinConfig{Mode: machine.PinInput})

	if released {
		debug.Log("SDA: HIGH (ok)")
	} else {
		debug.Log("SDA: LOW (STILL STUCK)")
	}

	if err := machine.I2C0.Configure(machine.I2CConfig{}); err != nil {
		debug.Log("FATAL: I2C: " + err.Error())
		proc.DisableWatchdog()
		blinkForever()
	}
	debug.Log("I2C: ok")
	proc.PetWatchdog()

	rtc, err := ds3231.Probe(machine.I2C0, uint8(machine.D12))
	if err != nil {
		debug.Log("FATAL: DS3231: " + err.Error())
		proc.DisableWatchdog()
		blinkForever()
	}
	debug.Log("DS3231: ok")
	proc.PetWatchdog()

	// Summary line for quick scanning across many boot cycles.
	wdt := isWDTReset()
	if wdt && stuck {
		debug.Log("** RECOVERED: WDT reset + stuck bus -> DS3231 OK **")
	} else if wdt {
		debug.Log("** WDT reset, bus was idle (WDT missed mid-transaction — keep running) **")
	}

	// --- Phase 1: tight ReadTime loop, petting watchdog. ---
	// Hit the reset button during this window to test recovery
	// from an external reset mid-transaction.
	const n = 500
	debug.Log("")
	debug.Log("--- phase 1: " + strconv.Itoa(n) + " reads (hit reset to test) ---")

	errs := 0
	for i := 1; i <= n; i++ {
		_, err := rtc.ReadTime()
		if err != nil {
			errs++
		}
		if i%100 == 0 {
			proc.PetWatchdog()
			debug.Log("  " + strconv.Itoa(i) + "/" + strconv.Itoa(n) + " (" + strconv.Itoa(errs) + " errs)")
		}
	}
	proc.PetWatchdog()
	debug.Log("phase 1: " + strconv.Itoa(errs) + " errors")

	// --- Phase 2: tight ReadTime loop WITHOUT petting watchdog. ---
	// The ~8s WDT timeout fires mid-I2C-transaction, resetting
	// the MCU. The next boot tests whether RecoverI2C unsticks the bus.
	debug.Log("")
	debug.Log("--- phase 2: WDT reset in ~8s ---")
	proc.PetWatchdog()

	count := 0
	for {
		rtc.ReadTime()
		count++
		if count%5000 == 0 {
			debug.Log("  " + strconv.Itoa(count) + " reads...")
		}
	}
}

func blinkForever() {
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
	for {
		machine.LED.High()
		wait.For(250 * time.Millisecond)
		machine.LED.Low()
		wait.For(250 * time.Millisecond)
	}
}
