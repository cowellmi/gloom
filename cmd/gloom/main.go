package main

import (
	"errors"
	"machine"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/power"
	"github.com/cowellmi/gloom/internal/rtc/ds3231"
	"github.com/cowellmi/gloom/internal/rtc/pcf8523"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/serial"
	"github.com/cowellmi/gloom/internal/wait"
)

func main() {
	// Keep track of non-fatal init issues for deferred logging.
	var initErrs []error
	var initWarns []error

	// Initialise MCU: debug UART, watchdog, chip-specific setup.
	// Defined in the build-tagged board file (e.g. main_feather_m0.go).
	proc := initMCU()

	// Start with debug-friendly defaults (fake sensor, serial on).
	// Board-specific defaults set pin candidates only.
	cfg := config.Default()
	boardDefaults(&cfg)

	// TODO: probe Blues Notecard on I2C 0x17.
	// If found, read config from env vars: blues.readConfig(&cfg).
	// This would override sensors (removing "fake"), intervals, etc.
	// with the Blueshub env vars values.

	proc.PetWatchdog()

	// --- Power rails ---
	//
	// Power rails are a compile-time board decision via boardPower()
	// (build-tagged). On Hypnos, this enables the D5 3.3V core rail
	// and D6 5V sensor rail before any peripheral probing. Builds
	// with the no_hypnos tag return nil, skipping rail control.
	var rails hal.Rails
	if r := boardPower(); len(r) > 0 {
		rails = power.NewController(r...)
	}

	proc.PetWatchdog()

	// Setup I2C after power rails are up so peripherals behind
	// MOSFET switches (e.g. DS3231 on Hypnos) don't pull the bus
	// low through ESD diodes during configuration.
	if err := machine.I2C0.Configure(machine.I2CConfig{}); err != nil {
		fatal(err)
	}

	// --- RTC probe ---
	//
	// Try DS3231 first (Hypnos).
	var clock hal.RTC
	ds, err := ds3231.Probe(machine.I2C0, cfg.RTCWakePin)
	if err != nil {
		proc.PetWatchdog()

		initWarns = append(initWarns, err)
		// Then try PCF8523 (Adalogger).
		pcf, err := pcf8523.Probe(machine.I2C0, cfg.RTCWakePin)
		if err != nil {
			initWarns = append(initWarns, err)
			// No RTC — System will use time.Now() and idle sleep.
		} else {
			clock = pcf
		}
	} else {
		clock = ds
	}

	proc.PetWatchdog()

	// --- SD card probe ---
	//
	// Probe ALL configured CS pins. Each working card is collected.
	// In the future, multiple cards could back separate file sinks
	// for redundancy in remote deployments. For now, the first
	// card found is used for config loading and file output.
	type sdEntry struct {
		card *sdcard.Card
		cs   uint8
	}
	var cards []sdEntry
	for _, cs := range cfg.SDCSPins {
		proc.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(
			machine.SPI0,
			machine.SPI0_SCK_PIN,
			machine.SPI0_SDO_PIN,
			machine.SPI0_SDI_PIN,
			machine.Pin(cs),
		)
		if err != nil {
			initWarns = append(initWarns, errors.New("sdcard: CS pin "+pin+": "+err.Error()))
			continue
		}
		cards = append(cards, sdEntry{card: c, cs: cs})
	}

	// Primary card for config and file sink.
	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	proc.PetWatchdog()

	// --- Config loading ---
	//
	// TODO: once Blues Notecard is implemented, config is loaded
	// from env vars above. On success, cache to SD card config.ini.
	// For now, load from SD card if available.
	if card != nil {
		raw, err := card.ReadFile("config.ini")
		if err != nil {
			// No config file — seed a default so the user has a
			// template to edit.
			if wErr := card.WriteFile("config.ini", []byte(config.DefaultINI)); wErr != nil {
				initErrs = append(initErrs, wErr)
			}
		} else if raw != nil {
			if err := config.Parse(raw, &cfg); err != nil {
				initErrs = append(initErrs, err)
			}
		}
	}

	cfg.HeartbeatInterval = 3 * time.Second

	proc.PetWatchdog()

	// --- Build system ---
	sys := hal.NewSystem(proc, clock, rails)

	// --- Logger + serial sinks ---

	logger := log.NewLogger()

	// Read RTC time early so the logger and file sink agree on the
	// date from the first write.
	now := time.Now()
	if t, err := sys.ReadTime(); err == nil {
		now = t
	}
	logger.SetTime(now)

	var uartSink, usbSink *serial.Sink
	if cfg.SerialEnabled {
		// USB-CDC
		if err := machine.Serial.Configure(machine.UARTConfig{
			BaudRate: 115200,
		}); err != nil {
			initErrs = append(initErrs, err)
		}

		// Debug UART was configured at startup by initMCU.
		uartSink = serial.NewSink(debugWriter())
		usbSink = serial.NewSink(machine.Serial)

		logger.AddSink(uartSink, log.LevelDebug)
		logger.AddSink(usbSink, log.LevelDebug)
	}

	proc.PetWatchdog()

	// --- File sink (only if SD card available) ---

	var fileSink *file.Sink
	if card != nil {
		if err := card.Mkdir("data"); err != nil {
			initErrs = append(initErrs, err)
		}
		if err := card.Mkdir("logs"); err != nil {
			initErrs = append(initErrs, err)
		}

		opener := func(name string) (file.AppendFile, error) {
			return card.OpenAppend(name)
		}
		var fileErr error
		fileSink, fileErr = file.New("sd", opener, file.FileSpec{
			Dir: "data",
			Ext: ".csv",
		}, file.FileSpec{
			Dir: "logs",
			Ext: ".log",
		}, now)
		if fileErr != nil {
			initErrs = append(initErrs, fileErr)
		} else {
			logger.AddSink(fileSink, log.LevelDebug)
		}
	}

	proc.PetWatchdog()

	// --- Resolve sensors ---

	var devices []sensor.Device
	for _, id := range cfg.Sensors {
		newDevice, ok := sensorRegistry[id]
		if !ok {
			initErrs = append(initErrs, errors.New("unknown sensor: "+id))
			continue
		}
		devices = append(devices, newDevice())
	}

	// --- LED ---

	var ledOn, ledOff func()
	if cfg.LedEnabled {
		machine.LED.Configure(machine.PinConfig{Mode: machine.PinOutput})
		ledOn = func() { machine.LED.High() }
		ledOff = func() { machine.LED.Low() }
	}

	// --- Report init warnings and errors ---

	for _, w := range initWarns {
		logger.Warn("init: " + w.Error())
	}
	for _, e := range initErrs {
		logger.Error("init: " + e.Error())
	}

	// --- Boot banner ---

	logger.Debug("mcu: " + proc.Identifier())

	if clock != nil {
		logger.Debug("rtc: " + clock.Identifier())
	} else {
		logger.Debug("rtc: none")
	}

	if len(cards) > 0 {
		sd := "sd:"
		for _, e := range cards {
			sd += " CS " + strconv.Itoa(int(e.cs))
		}
		logger.Debug(sd)
	} else {
		logger.Debug("sd: none")
	}

	proc.PetWatchdog()

	// --- Manager ---

	man := manager.New(sys, cfg, devices, logger)
	man.EnableLED(ledOn, ledOff)
	man.EnableWatchdog(proc.PetWatchdog)

	if cfg.SerialEnabled {
		man.AddRecorder(uartSink)
		man.AddRecorder(usbSink)
	}
	if fileSink != nil {
		man.AddRecorder(fileSink)
	}

	// Watchdog is already running — enter the main loop.
	man.Run()
}

// petWatchdog is set after the watchdog is enabled so that fatal()
// can pet it during the blink loop. Nil before initMCU.
var petWatchdog func()

// fatal prints the error to the debug output and blinks the LED
// forever to signal a hard failure when no serial monitor is
// connected. If the watchdog is running it is petted each blink
// cycle to prevent a reset — this is a permanent halt, not a
// transient hang.
func fatal(err error) {
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
