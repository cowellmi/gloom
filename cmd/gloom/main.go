//go:build tinygo

package main

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/led"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/rtc/ds3231"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/vbat"
	"github.com/cowellmi/gloom/internal/sink"
	"github.com/cowellmi/gloom/internal/sleeper"
	"github.com/cowellmi/gloom/internal/wait"
)

var ProductUID string

func main() {
	var initErrs []error
	var initWarns []error

	// Board
	board := initBoard()
	board.LED.On()
	board.MCU.PaintStack()
	board.MCU.EnableWatchdog()
	debug.W = board.Serial

	// Devices
	dev := initDevices()

	// Power rails
	rails := initRails()
	if rails != nil {
		debug.Log("power-cycling rails...")
		rails.Power(hal.RailsOff)
		wait.For(250 * time.Millisecond)
		rails.Power(hal.RailsCore)
		board.MCU.PetWatchdog()
		wait.For(2 * time.Second)
	}

	board.MCU.PetWatchdog()
	debug.Log("configuring I2C...")

	if err := board.MCU.ConfigureI2C(board.I2C.SDA, board.I2C.SCL); err != nil {
		board.MCU.DisableWatchdog()
		fatal(err, board.LED)
	}

	board.MCU.PetWatchdog()
	debug.Log("probing RTC...")

	// RTC
	var clock hal.RTC
	ds, err := ds3231.Probe(board.I2C.Bus)
	if err != nil {
		board.MCU.PetWatchdog()
		initWarns = append(initWarns, err)
	} else {
		clock = ds
	}

	board.MCU.PetWatchdog()
	debug.Log("probing Notecard...")

	// Blues Notecard
	nc, err := notecard.New(board.I2C.Bus.Tx)
	if err != nil {
		initWarns = append(initWarns, err)
	} else {
		debug.Log("notecard: hub.sync")
		err := nc.Request(map[string]any{
			"req": "hub.sync",
		})
		if err != nil {
			initErrs = append(initErrs, err)
		}
	}

	runtime.GC()
	board.MCU.PetWatchdog()
	debug.Log("probing SD card...")

	// SD Card
	type sdEntry struct {
		card *sdcard.Card
		cs   hal.Pin
	}
	var cards []sdEntry
	for _, cs := range dev.SDChipSelectPins {
		board.MCU.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(board.SPI.Bus, board.SPI.SCK, board.SPI.SDO, board.SPI.SDI, cs)
		if err != nil {
			if len(cards) == 0 {
				initWarns = append(initWarns, errors.New("CS "+pin+": "+err.Error()))
			}
		} else {
			cards = append(cards, sdEntry{card: c, cs: cs})
		}
	}

	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	board.MCU.PetWatchdog()

	// Load config
	cfg := config.Default(board.LED.Pin(), board.Sensors)

	if nc != nil {
		debug.Log("loading config from Notehub...")
		rsp, rErr := nc.RequestResponse(map[string]any{
			"req":  "note.get",
			"file": "config.db",
			"note": "config",
		})
		switch {
		case rErr == nil:
			if body, ok := rsp["body"].(map[string]any); ok {
				if pErr := config.ParseMap(&cfg, body); pErr != nil {
					initErrs = append(initErrs, errors.New("config: "+pErr.Error()))
				}
			}
		case notecard.IsNotFound(rErr):
			debug.Log("sending default config to Notehub...")
			if _, wErr := nc.RequestResponse(map[string]any{
				"req":  "note.add",
				"file": "config.db",
				"note": "config",
				"body": cfg.MarshalMap(),
				"sync": true,
			}); wErr != nil {
				initErrs = append(initErrs, errors.New("notecard: config.db: "+wErr.Error()))
			}
		default:
			initErrs = append(initErrs, errors.New("notecard: config.db: "+rErr.Error()))
		}
		board.MCU.PetWatchdog()
	} else if card != nil {
		debug.Log("loading config from SD card...")
		raw, err := card.ReadFile("CONFIG.INI")
		if err == nil {
			if pErr := config.ParseINI(raw, &cfg); pErr != nil {
				if joined, ok := pErr.(interface{ Unwrap() []error }); ok {
					for _, e := range joined.Unwrap() {
						initErrs = append(initErrs, errors.New("config: "+e.Error()))
					}
				} else {
					initErrs = append(initErrs, errors.New("config: "+pErr.Error()))
				}
			}
		} else { // TODO: check for specific file not found err
			// No CONFIG.INI on SD card; attempt to make one.
			if ini, mErr := cfg.MarshalINI(); mErr != nil {
				initErrs = append(initErrs, errors.New("config: "+mErr.Error()))
			} else if wErr := card.WriteFile("CONFIG.INI", ini); wErr != nil {
				initErrs = append(initErrs, errors.New("sd: "+wErr.Error()))
			}
		}
	}

	if cfg.HeartbeatLedPin != hal.NoPin {
		board.LED = led.New(cfg.HeartbeatLedPin)
	}

	// Read the current time once — used for both the file sink
	// (daily rotation key) and the logger's initial timestamp.
	now := time.Now()
	if clock != nil {
		if t, err := clock.ReadTime(); err != nil {
			initErrs = append(initErrs, errors.New("rtc: "+err.Error()))
		} else {
			now = t
		}
	}

	// Setup log and recorder sinks.
	logger := log.NewLogger(now)

	// Automatically add serial sink.
	serialSink := sink.NewSerial(board.Serial)
	logger.AddSink(serialSink, config.LogLevelDebug)
	dataSinks := []sink.DataSink{serialSink}

	if nc != nil {
		notehubSink := sink.NewNotehubSink(nc, "data.qo", "log.qo")
		dataSinks = append(dataSinks, notehubSink)
		logger.AddSink(notehubSink, cfg.BluesLogLevel)
		board.MCU.PetWatchdog()
	}

	if card != nil {
		if err := card.Mkdir("GLOOM"); err != nil {
			initErrs = append(initErrs, errors.New("sd: "+err.Error()))
		}
		opener := func(name string) (sink.AppendFile, error) {
			return card.OpenAppend(name)
		}
		sdCardFileSink, err := sink.NewRotaryFileSink(opener, sink.FileSpec{
			Dir: "GLOOM",
			Ext: ".CSV",
		}, sink.FileSpec{
			Dir: "GLOOM",
			Ext: ".LOG",
		}, now)
		if err != nil {
			initErrs = append(initErrs, errors.New("file: "+err.Error()))
		} else {
			dataSinks = append(dataSinks, sdCardFileSink)
			logger.AddSink(sdCardFileSink, cfg.SDLogLevel)
		}
		board.MCU.PetWatchdog()
	}

	// Report init warnings and errors
	for _, w := range initWarns {
		if joined, ok := w.(interface{ Unwrap() []error }); ok {
			for _, sub := range joined.Unwrap() {
				logger.Warn("init: " + sub.Error())
			}
		} else {
			logger.Warn("init: " + w.Error())
		}
	}
	for _, e := range initErrs {
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			for _, sub := range joined.Unwrap() {
				logger.Error("init: " + sub.Error())
			}
		} else {
			logger.Error("init: " + e.Error())
		}
	}

	// Sensors
	var sensors []sensor.Sensor
	for _, id := range cfg.SampleSensors {
		switch id {
		case "vbat":
			if board.ADCPin != hal.NoPin {
				sensors = append(sensors, vbat.NewDevice(board.ADCPin))
			} else {
				logger.Warn("sensors: vbat: no ADC pin")
			}
		default:
			logger.Warn("sensors: unknown id: " + id)
		}
		board.MCU.PetWatchdog()
	}

	board.LED.Off()

	// Boot banner
	logger.Debug("mcu: " + board.MCU.Identifier())

	if clock != nil {
		logger.Debug("rtc: " + clock.Identifier())
	} else {
		logger.Debug("rtc: NONE")
	}

	if len(cards) > 0 {
		sd := "sd:"
		for _, e := range cards {
			sd += " CS " + strconv.Itoa(int(e.cs))
		}
		logger.Debug(sd)
	} else {
		logger.Debug("sd: NONE")
	}

	if nc != nil {
		logger.Debug("notecard: " + nc.UID)
	} else {
		logger.Debug("notecard: NONE")
	}

	var bootBuf [256]byte
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := bootBuf[:0]
	b = fmtbuf.Append(b, "mem: heap_sys=")
	b = fmtbuf.AppendUint(b, ms.HeapSys/1024, 10)
	b = fmtbuf.Append(b, "KB")
	if ss := board.MCU.StackSize(); ss > 0 {
		b = fmtbuf.Append(b, " stack_size=")
		b = fmtbuf.AppendUint(b, uint64(ss/1024), 10)
		b = fmtbuf.Append(b, "KB")
	}
	logger.Debug(string(b))

	board.MCU.PetWatchdog()

	// Manager
	interruptPins := append(dev.InterruptPins, cfg.SampleExtPin)
	sleeper := sleeper.New(board.MCU, clock, rails, interruptPins)
	man := manager.New(sleeper, cfg, sensors, dataSinks, logger)

	// Validate there is at least one wake source.
	if cfg.SampleInterval <= 0 && cfg.SampleExtPin == hal.NoPin && cfg.HeartbeatInterval <= 0 {
		err := errors.New("config: no wake sources configured (needs sample_interval, sample_ext_pin, or heartbeat_interval)")
		logger.Error(err.Error())
		fatal(err, board.LED)
	}

	if cfg.HeartbeatInterval > 0 && cfg.HeartbeatLedPin != hal.NoPin {
		man.SetBlinkLED(board.LED.Blink)
	}
	man.EnableWatchdog(board.MCU.PetWatchdog)
	man.SetStackMonitor(board.MCU.StackUsed)

	if clock != nil {
		if t, err := clock.ReadTime(); err == nil {
			now = t
		}
	} else {
		now = time.Now()
	}
	man.Run(now)
}

// fatal blinks the LED forever to signal a hard failure when no
// serial monitor is connected.
func fatal(err error, led hal.LED) {
	debug.Log("FATAL: " + err.Error())
	for {
		led.Blink()
	}
}
