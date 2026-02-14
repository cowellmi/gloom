package main

import (
	"errors"
	"machine"
	"time"

	"github.com/cowellmi/gloom/internal/wait"

	"github.com/cowellmi/gloom/internal/boards/hypnos"
	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/mcu/samd21"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/serial"
)

// UART0 pins on SERCOM0. D0 (PA11) and D1 (PA10) are the standard
// Feather M0 RX/TX header pins, freeing D10/D11 for SD card CS.
const (
	UART_TX_PIN = machine.D1
	UART_RX_PIN = machine.D0
)

func main() {
	// Serial sinks.
	var uartSink, usbSink *serial.Sink

	// Keep track of non-fatal init errors for deferred logging.
	var initErrs []error

	println("hello world")

	// Configure UART0 early so fatal errors are visible on the
	// UART monitor even before the full serial setup.
	configureUART0(115200)

	// Setup I2C with default config.
	err := machine.I2C0.Configure(machine.I2CConfig{})
	if err != nil {
		fatal(err)
	}

	// Load the ATSAMD21 (from Feather M0). It implements MCU.
	proc := samd21.New()

	// Enable watchdog early so SPI/I2C hangs during probe (e.g.
	// corrupted SD card) cause a reset instead of a permanent hang.
	// Probe and subsequent init steps pet the watchdog at strategic
	// points to avoid tripping it during normal startup.
	proc.EnableWatchdog()
	petWatchdog = proc.PetWatchdog

	// Load Hypnos board. Probe detects the RTC and SD card reader.
	board, err := hypnos.Probe(
		machine.I2C0,
		machine.SPI0,
		machine.SPI0_SCK_PIN,
		machine.SPI0_SDO_PIN,
		machine.SPI0_SDI_PIN,
		proc,
	)
	if err != nil {
		fatal(err)
	}

	proc.PetWatchdog()

	logger := log.NewLogger()
	card := board.Card

	// Load config from SD card. If missing, seed a default config.ini
	// so the user has a template to edit.
	cfg := config.Default()
	raw, err := card.ReadFile("config.ini")
	if err != nil {
		if wErr := card.WriteFile("config.ini", []byte(config.DefaultINI)); wErr != nil {
			initErrs = append(initErrs, wErr)
		}
	} else if raw != nil {
		if err := config.Parse(raw, &cfg); err != nil {
			initErrs = append(initErrs, err)
		}
	}

	proc.PetWatchdog()

	// Create directories for daily-rotating data and log files.
	if err := card.Mkdir("data"); err != nil {
		initErrs = append(initErrs, err)
	}
	if err := card.Mkdir("logs"); err != nil {
		initErrs = append(initErrs, err)
	}

	// Create file sink with daily rotation. The opener wraps
	// card.OpenAppend so the sink can open new date-stamped files
	// as needed (e.g. data/20260214.csv, logs/20260214.log).
	opener := func(name string) (file.AppendFile, error) {
		return card.OpenAppend(name)
	}
	now := time.Now()
	if t, err := board.ReadTime(); err == nil {
		now = t
	}
	fileSink, fileErr := file.New("sd", opener, file.FileSpec{
		Dir: "data",
		Ext: ".csv",
	}, file.FileSpec{
		Dir: "logs",
		Ext: ".log",
	}, now)
	if fileErr != nil {
		initErrs = append(initErrs, fileErr)
	}

	proc.PetWatchdog()

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

		// UART0 was configured at startup for early error output.
		uartSink = serial.NewSink(UART0)
		usbSink = serial.NewSink(machine.Serial)

		logger.AddSink(uartSink, log.LevelDebug)
		logger.AddSink(usbSink, log.LevelDebug)
	}

	logger.AddSink(fileSink, log.LevelDebug)

	// Report init errors through logger sinks.
	for _, e := range initErrs {
		logger.Error("init: " + e.Error())
	}

	man := manager.New(board, cfg, devices, logger)
	man.EnableLED(ledOn, ledOff)

	// Register recorders for measurement output.
	if cfg.SerialEnabled {
		man.AddRecorder(uartSink)
		man.AddRecorder(usbSink)
	}
	man.AddRecorder(fileSink)

	// Watchdog is already running -- enter the main loop.
	man.Run()
}

// petWatchdog is set after the watchdog is enabled so that fatal()
// can pet it during the blink loop. Nil before EnableWatchdog.
var petWatchdog func()

// fatal prints the error to USB-CDC and blinks the LED forever to
// signal a hard failure when no serial monitor is connected. If the
// watchdog is running it is petted each blink cycle to prevent a
// reset -- this is a permanent halt, not a transient hang.
func fatal(err error) {
	msg := "fatal: " + err.Error()
	println(msg)
	UART0.Write([]byte(msg + "\r\n"))
	machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
	for {
		if petWatchdog != nil {
			petWatchdog()
		}
		machine.LED.High()
		wait.For(250 * time.Millisecond)
		machine.LED.Low()
		wait.For(250 * time.Millisecond)
	}
}
