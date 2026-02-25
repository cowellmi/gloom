//go:build tinygo

package main

import (
	"errors"
	"runtime"
	"strconv"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/fallback"
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
)

var ProductUID string

func main() {
	var initErrs []error
	var initWarns []error

	board := initBoard()
	board.MCU.PaintStack()
	board.MCU.EnableWatchdog()

	wing := initWing()
	board.MCU.PetWatchdog()

	var statusLED hal.LED = fallback.LED{}
	if board.LEDPin != hal.NoPin {
		statusLED = led.New(board.LEDPin)
		statusLED.On()
	}

	debug.W = board.Serial
	debug.Log("version: 0.0")

	err := board.MCU.ConfigureI2C(board.SDA, board.SCL)
	if err != nil {
		board.MCU.DisableWatchdog()
		fatal(err, statusLED)
	}
	board.MCU.PetWatchdog()

	var rtc hal.Clock = fallback.RTC{}
	if ds, rErr := ds3231.Probe(board.I2C); rErr != nil {
		initWarns = append(initWarns, rErr)
	} else {
		rtc = ds
	}

	nc, ncErr := notecard.New(board.I2C.Tx)
	if ncErr != nil {
		initWarns = append(initWarns, ncErr)
	}
	board.MCU.PetWatchdog()
	runtime.GC()

	cfg := config.Default(board.LEDPin, board.Sensors, wing.SDChipSelectPins, wing.InterruptPins)
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
	}
	board.MCU.PetWatchdog()

	debug.Log("probing SD card...")

	// SD Card
	type sdEntry struct {
		card *sdcard.Card
		cs   hal.Pin
	}
	var cards []sdEntry
	for _, cs := range cfg.SDChipSelectPins {
		board.MCU.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(board.SPI, board.SCK, board.SDO, board.SDI, cs)
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

	if nc == nil && card != nil {
		raw, err := card.ReadFile("CONFIG.INI")
		if err == nil {
			debug.Log("loading config from SD card...")
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
			debug.Log("writing default config to SD card...")
			if ini, mErr := cfg.MarshalINI(); mErr != nil {
				initErrs = append(initErrs, errors.New("config: "+mErr.Error()))
			} else if wErr := card.WriteFile("CONFIG.INI", ini); wErr != nil {
				initErrs = append(initErrs, errors.New("sd: "+wErr.Error()))
			}
		}
	}

	if cfg.HeartbeatLedPin != hal.NoPin {
		statusLED = led.New(cfg.HeartbeatLedPin)
	}

	now, err := rtc.ReadTime()
	if err != nil {
		initErrs = append(initErrs, err)
		rtc = fallback.RTC{}
	}

	logger := log.NewLogger(now)

	serialSink := sink.NewSerial(board.Serial)
	logger.AddSink(serialSink, config.LogLevelDebug)
	dataSinks := []sink.DataSink{serialSink}

	if nc != nil {
		notehubSink := sink.NewNotehubSink(nc, "data.qo", "log.qo")
		dataSinks = append(dataSinks, notehubSink)
		logger.AddSink(notehubSink, cfg.LogLevelBlues)
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
			logger.AddSink(sdCardFileSink, cfg.LogLevelSD)
		}
		board.MCU.PetWatchdog()
	}

	for _, err := range initWarns {
		logger.LogError(config.LogLevelWarn, err, "init: ")
	}
	for _, err := range initErrs {
		logger.LogError(config.LogLevelError, err, "init: ")

	}

	var sensors []sensor.Sensor
	for _, id := range cfg.SampleSensors {
		switch id {
		case "vbat":
			if board.ADCPin != hal.NoPin {
				sensors = append(sensors, vbat.NewDevice(board.ADCPin))
			} else {
				logger.Log(config.LogLevelWarn, "sensors: vbat: no ADC pin")
			}
		default:
			logger.Log(config.LogLevelWarn, "sensors: unknown id: "+id)
		}
		board.MCU.PetWatchdog()
	}

	statusLED.Off()

	logger.Log(config.LogLevelInfo, "mcu: "+board.MCU.Identifier())
	logger.Log(config.LogLevelInfo, "rtc: "+rtc.Identifier())
	logger.Log(config.LogLevelInfo, "rails: "+wing.Rails.Identifier())

	if len(cards) > 0 {
		sd := "sd:"
		for _, e := range cards {
			sd += " CS " + strconv.Itoa(int(e.cs))
		}
		logger.Log(config.LogLevelDebug, sd)
	} else {
		logger.Log(config.LogLevelDebug, "sd: none")
	}

	if nc != nil {
		logger.Log(config.LogLevelDebug, "notecard: "+nc.UID)
	} else {
		logger.Log(config.LogLevelDebug, "notecard: none")
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
	logger.Log(config.LogLevelDebug, string(b))

	board.MCU.PetWatchdog()

	// Manager
	sleeper := sleeper.New(board.MCU, rtc, wing.Rails, cfg.InterruptPins)
	man := manager.New(sleeper, wing.Rails, statusLED, cfg, sensors, dataSinks, logger)

	// Validate there is at least one wake source.
	if cfg.SampleInterval <= 0 && cfg.HeartbeatInterval <= 0 && len(cfg.InterruptPins) == 0 {
		err := errors.New("config: no wake sources configured")
		logger.LogError(config.LogLevelError, err, "")
		board.MCU.DisableWatchdog()
		fatal(err, statusLED)
	}

	man.EnableWatchdog(board.MCU.PetWatchdog)
	man.SetStackMonitor(board.MCU.StackUsed)

	now, err = rtc.ReadTime()
	if err != nil {
		rtc = fallback.RTC{}
		logger.LogError(config.LogLevelError, err, "rtc: ")
	}
	man.Run(now)
}

// fatal blinks the LED forever to signal a hard failure when no
// serial monitor is connected.
func fatal(err error, statusLED hal.LED) {
	debug.Log("FATAL: " + err.Error())
	for {
		statusLED.Blink()
	}
}
