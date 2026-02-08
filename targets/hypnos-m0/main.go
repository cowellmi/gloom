package main

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/hal/base"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
	"github.com/cowellmi/gloom/internal/boards/hypnos"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/serial"
)

func main() {
	// Keep track of non-fatal init errors for println reporting.
	var initErrs []error

	// Setup I2C.
	err := machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 100e3, // 100 kHz
	})
	if err != nil {
		println("fatal:", err.Error())
		return
	}

	// Probe hardware stack. Keep the concrete board reference for
	// storage access (SD card); the hal.Platform interface is used
	// by the manager for clock/sleep only.
	proc := samd21.New()
	var sys hal.Platform
	board, err := hypnos.Probe(machine.I2C0, proc)
	if err != nil {
		initErrs = append(initErrs, err)
		sys = base.New()
	} else {
		sys = board
	}

	// Parse config file (requires storage -- only available on Hypnos).
	// TODO: read config from SD card via board.SD once implemented.
	cfg := config.Default()

	// Resolve sensor IDs from config to actual devices.
	var devices []sensor.Device
	for _, id := range cfg.Sensors {
		newDevice, ok := sensorRegistry[id]
		if !ok {
			initErrs = append(initErrs, errors.New("unknown sensor: "+id))
			continue
		}
		devices = append(devices, newDevice())
	}

	// Configure LED.
	var ledOn, ledOff func()
	if cfg.EnableMachineLED {
		machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
		ledOn = func() { machine.LED.High() }
		ledOff = func() { machine.LED.Low() }
	}

	// Configure serial.
	if cfg.SerialEnabled {
		err = machine.Serial.Configure(machine.UARTConfig{
			BaudRate: 115200,
		})
		if err != nil {
			initErrs = append(initErrs, err)
		}
	}

	// Build serial-wait callback.
	var waitFn func() error
	if cfg.WaitForSerial {
		waitFn = waitForSerial(cfg.MaxWaitForSerial)
		if err := waitFn(); err != nil {
			initErrs = append(initErrs, err)
		}
	}

	// Report init errors via println (sinks aren't ready yet).
	for _, e := range initErrs {
		println("init:", e.Error())
	}

	// Create manager (all runtime output goes through sinks).
	man := manager.New(sys, cfg, devices)
	man.OnLED(ledOn, ledOff)
	man.OnWaitForSerial(waitFn)

	// Register sinks.
	if cfg.SerialEnabled {
		man.AddSink(serial.New(machine.Serial))
	}
	// Register SD file sink if the board has an SD card.
	if board != nil && board.SD != nil {
		dataW, err := board.SD.OpenWriter("data.csv")
		if err != nil {
			println("init: sd data:", err.Error())
		}
		logW, err := board.SD.OpenWriter("log.txt")
		if err != nil {
			println("init: sd log:", err.Error())
		}
		if dataW != nil || logW != nil {
			man.AddSink(file.New("sd", dataW, logW, board.SD.Sync))
		}
	}

	man.Run()
}

const waitForSerialInterval = 100 * time.Millisecond

func waitForSerial(maxWait time.Duration) func() error {
	return func() error {
		// After standby wake, the USB CDC line state (DTR) carries
		// a stale value from before sleep. Pause to let the host
		// re-enumerate USB and the terminal reopen the port so that
		// a fresh SET_CONTROL_LINE_STATE overwrites the stale DTR
		// before we check it.
		time.Sleep(time.Second)

		var waited time.Duration
		for !machine.Serial.DTR() {
			if waited > maxWait {
				return errors.New("wait for serial timed out")
			}
			time.Sleep(waitForSerialInterval)
			waited += waitForSerialInterval
		}
		return nil
	}
}
