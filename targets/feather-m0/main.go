package main

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/boards/hypnos"
	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink/serial"
)

// UART1 pins
const (
	UART_TX_PIN = machine.D10
	UART_RX_PIN = machine.D11
)

func main() {
	// Serial sinks.
	var UART1, USBCDC *serial.Sink

	// Keep track of non-fatal init errors for deferred logging.
	var initErrs []error

	println("hello world")

	// Setup I2C with default config.
	err := machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		println("fatal:", err.Error())
		return
	}

	// Load the ATSAMD21 (from Feather M0). It implements MCU.
	proc := samd21.New()

	// Load Hypnos board.
	var sys hal.Platform
	board, err := hypnos.Probe(machine.I2C0, proc)
	if err != nil {
		// Unable to load Hypnos. Using fallback platform.
		initErrs = append(initErrs, err)
		sys = &hal.Fallback{}
	} else {
		// Successfully loaded Hypnos board. Set as system platform.
		sys = board
	}

	logger := log.NewLogger()

	// Load default config then overwrite with values read from storage device.
	cfg := config.Default()

	// Update samples manually for testing.
	cfg.SampleInterval = 7 * time.Second
	cfg.HeartbeatInterval = 11 * time.Second

	// TODO: read config file from SD card on Hypnos. Ideally we can detect the
	// Hypnos board version during hypnos.Probe to determine chip select pin.
	//
	// For future reference:
	// DEVICE     | CHIP SELECT PIN
	// Hypnos 3.3 | 11
	// Hypnos 3.2 | 10
	// Adalogger  | 4
	//
	// We will probably have storage interface and write implementations using
	// these three sd card readers.

	// Add dummy sensor for debugging.
	cfg.Sensors = []string{"fake"}

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
	if cfg.LedEnabled {
		machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
		ledOn = func() { machine.LED.High() }
		ledOff = func() { machine.LED.Low() }
	}

	if cfg.SerialEnabled {
		// USB-CDC
		err = machine.Serial.Configure(machine.UARTConfig{
			BaudRate: 115200,
		})
		if err != nil {
			initErrs = append(initErrs, err)
		}

		// UART1
		err = machine.UART1.Configure(machine.UARTConfig{
			BaudRate: 115200,
			TX:       UART_TX_PIN,
			RX:       UART_RX_PIN,
		})
		if err != nil {
			initErrs = append(initErrs, err)
		}

		UART1 = serial.NewSink(machine.UART1)
		sinkUSBCDC := serial.NewSink(machine.Serial)

		logger.AddSink(UART1, log.LevelDebug)
		logger.AddSink(sinkUSBCDC, log.LevelDebug)
	}
	// TODO: register file sink with SD card reader/writer.

	// Report init errors through logger sinks.
	for _, e := range initErrs {
		logger.Error("init: " + e.Error())
	}

	// Create manager.
	man := manager.New(sys, cfg, devices, logger)
	man.EnableLED(ledOn, ledOff)

	// Register recorders for measurement output.
	if cfg.SerialEnabled {
		man.AddRecorder(UART1)
		man.AddRecorder(USBCDC)
	}
	// TODO: register file sink recorder with SD card manager.

	man.Run()
}
