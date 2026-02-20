package main

import (
	"errors"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/drivers/ds3231"
	"github.com/cowellmi/gloom/internal/drivers/pcf8523"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/power"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/serial"
	"github.com/cowellmi/gloom/internal/wait"
)

func main() {
	var initErrs []error
	var initWarns []error

	// Start with debug-friendly defaults (fake sensor, serial on).
	cfg := config.Default()

	// Initialise board: debug UART, USB-CDC, MCU, pin defaults.
	// Defined in the build-tagged board file (e.g. main_feather_m0.go).
	board := initBoard(&cfg)
	board.MCU.ConfigureLED(cfg.LedPin)
	board.MCU.LedOn() // Signals start of init sequence.
	board.MCU.EnableWatchdog()

	// Setup debug logger
	debug.W = board.UART
	debug.Log("loading defaults...")

	// TODO: probe Blues Notecard on I2C 0x17.
	// If found, read config from env vars: blues.readConfig(&cfg).
	// This would override sensors (removing "fake"), intervals, etc.
	// with the Blueshub env vars values.

	board.MCU.PetWatchdog()
	debug.Log("powering rails...")

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

	board.MCU.PetWatchdog()
	debug.Log("configuring I2C...")

	// Configure I2C with bus recovery. If the MCU reset (watchdog,
	// brownout) while a slave was mid-transaction, the bit-banged
	// recovery clocks SCL to let it release before configuring the
	// peripheral.
	if err := board.MCU.ConfigureI2C(board.SDA, board.SCL); err != nil {
		board.MCU.DisableWatchdog()
		fatal(err, board.MCU)
	}

	board.MCU.PetWatchdog()
	debug.Log("probing rtc...")

	// --- RTC probe ---
	//
	// Try DS3231 first (Hypnos).
	var clock hal.RTC
	ds, err := ds3231.Probe(board.I2C, cfg.RTCWakePin)
	if err != nil {
		board.MCU.PetWatchdog()
		initWarns = append(initWarns, err)
		// Then try PCF8523 (Adalogger).
		pcf, err := pcf8523.Probe(board.I2C, cfg.RTCWakePin)
		if err != nil {
			initWarns = append(initWarns, err)
		} else {
			clock = pcf
		}
	} else {
		clock = ds
	}

	board.MCU.PetWatchdog()
	debug.Log("probing sd...")

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
		board.MCU.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(board.SPI.Bus, board.SPI.SCK, board.SPI.SDO, board.SPI.SDI, cs)
		if err != nil {
			initWarns = append(initWarns, errors.New("CS: "+pin+": "+err.Error()))
			continue
		}
		cards = append(cards, sdEntry{card: c, cs: cs})
	}

	// Primary card for config and file sink.
	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	board.MCU.PetWatchdog()

	// --- Config loading ---
	//
	// TODO: once Blues Notecard is implemented, config is loaded
	// from env vars above. On success, cache to SD card CONFIG.INI.
	// For now, load from SD card if available.
	//
	// NOTE: we use all caps for SD card filenames and directories
	// to support 8.3 file format (so we can disable LFN).
	if card != nil {
		raw, err := card.ReadFile("CONFIG.INI")
		if err != nil {
			// No config file — seed a default so the user has a
			// template to edit.
			wErr := card.WriteFile("CONFIG.INI", []byte(config.DefaultINI))
			if wErr != nil {
				initErrs = append(initErrs, wErr)
			}
		} else if raw != nil {
			if err := config.Parse(raw, &cfg); err != nil {
				initErrs = append(initErrs, err)
			}
		}
	}

	// Reconfigure LED with the (possibly overridden) pin.
	board.MCU.ConfigureLED(cfg.LedPin)

	board.MCU.PetWatchdog()

	// --- Build system ---
	sys := hal.NewSystem(board.MCU, clock, rails, cfg.SampleInterval, cfg.HeartbeatInterval)

	// --- Logger + serial sinks ---

	// Read RTC time early so the logger and file sink agree on the
	// date from the first write.
	now := time.Now()
	if t, err := sys.ReadTime(); err == nil {
		now = t
	}
	logger := log.NewLogger(now)

	var uartSink, usbSink *serial.Sink
	if cfg.SerialEnabled {
		uartSink = serial.NewSink(board.UART)
		usbSink = serial.NewSink(board.USBCDC)

		logger.AddSink(uartSink, log.LevelDebug)
		logger.AddSink(usbSink, log.LevelDebug)
	}

	board.MCU.PetWatchdog()

	// --- File sink (only if SD card available) ---

	var fileSink *file.Sink
	if card != nil {
		if err := card.Mkdir("DATA"); err != nil {
			initErrs = append(initErrs, err)
		}
		if err := card.Mkdir("LOGS"); err != nil {
			initErrs = append(initErrs, err)
		}

		opener := func(name string) (file.AppendFile, error) {
			return card.OpenAppend(name)
		}
		var fileErr error
		fileSink, fileErr = file.New("sd", opener, file.FileSpec{
			Dir: "DATA",
			Ext: ".CSV",
		}, file.FileSpec{
			Dir: "LOGS",
			Ext: ".LOG",
		}, now)
		if fileErr != nil {
			initErrs = append(initErrs, fileErr)
		} else {
			logger.AddSink(fileSink, log.LevelDebug)
		}
	}

	board.MCU.PetWatchdog()

	// --- Resolve sensors ---

	var devices []sensor.Device
	for _, id := range cfg.Sensors {
		newDevice, ok := sensorRegistry[id]
		if !ok {
			initErrs = append(initErrs, errors.New("unknown sensor: "+id))
			continue
		}
		devices = append(devices, newDevice())
		board.MCU.PetWatchdog()
	}

	board.MCU.LedOff() // Signals end of init sequence.

	// --- Report init warnings and errors ---

	for _, w := range initWarns {
		logger.Warn("init: " + w.Error())
	}
	for _, e := range initErrs {
		logger.Error("init: " + e.Error())
	}

	// --- Boot banner ---

	logger.Debug("mcu: " + board.MCU.Identifier())

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

	board.MCU.PetWatchdog()

	// --- Manager ---

	man := manager.New(sys, cfg, devices, logger)
	if cfg.LedEnabled {
		man.EnableLED(board.MCU.LedOn, board.MCU.LedOff)
	}
	man.EnableWatchdog(board.MCU.PetWatchdog)

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

// fatal blinks the LED forever to signal a hard failure when no
// serial monitor is connected. The caller must disable the watchdog
// before calling fatal.
func fatal(err error, mcu hal.MCU) {
	debug.Log("FATAL: " + err.Error())
	for {
		mcu.LedOn()
		wait.For(250 * time.Millisecond)
		mcu.LedOff()
		wait.For(250 * time.Millisecond)
	}
}
