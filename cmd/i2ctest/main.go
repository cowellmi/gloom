// Command i2ctest verifies I2C bus recovery on Hypnos hardware.
//
// It hammers DS3231 ReadTime in a tight loop without petting the
// watchdog. The ~8s WDT timeout fires mid-I2C-transaction, resetting
// the MCU. On the next boot, ConfigureI2C must unstick the bus so
// the DS3231 probes cleanly. The test halts with PASS once recovery
// from a stuck bus is confirmed.
//
// Flash and monitor:
//
//	make flash-i2ctest
//	make monitor
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
	wait.For(2 * time.Second)

	proc := initMCU()

	debug.Log("")
	debug.Log("=== I2C RECOVERY TEST ===")
	debug.Log("reset: " + resetCause())

	machine.D6.Configure(machine.PinConfig{Mode: machine.PinOutput})
	machine.D6.High() // active-high = ON
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

	if err := proc.ConfigureI2C(sda, scl); err != nil {
		debug.Log("I2C failed, WDT will retry: " + err.Error())
		select {}
	}
	debug.Log("I2C: ok")
	proc.PetWatchdog()

	rtc, err := ds3231.Probe(machine.I2C0, uint8(machine.D12))
	if err != nil {
		debug.Log("DS3231 failed, WDT will retry: " + err.Error())
		select {}
	}
	debug.Log("DS3231: ok")
	proc.PetWatchdog()

	if isWDTReset() && stuck {
		debug.Log("** RECOVERED: WDT reset + stuck bus -> DS3231 OK **")
		debug.Log("PASS")
		proc.DisableWatchdog()
		select {}
	}

	if isWDTReset() {
		debug.Log("WDT missed mid-transaction, retrying...")
	}

	// Tight ReadTime loop without petting the watchdog. The ~8s
	// WDT timeout fires mid-I2C-transaction, resetting the MCU.
	// The next boot tests whether ConfigureI2C unsticks the bus.
	// Do not press the reset button — external resets power-cycle
	// the DS3231 and clear the stuck bus before we can observe it.
	debug.Log("")
	debug.Log("Pending watchog reset...")
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
