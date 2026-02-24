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
	"github.com/cowellmi/gloom/internal/rtc/pcf8523"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/vbat"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/notehub"
	"github.com/cowellmi/gloom/internal/sink/serial"
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

	// Power rails
	rails := initRails()
	if rails != nil {
		debug.Log("power cycling rails...")
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
	debug.Log("probing rtc...")

	// RTC
	var clock hal.RTC
	ds, err := ds3231.Probe(board.I2C.Bus, board.RTCWakePin)
	if err != nil {
		board.MCU.PetWatchdog()
		initWarns = append(initWarns, err)
		pcf, err := pcf8523.Probe(board.I2C.Bus, board.RTCWakePin)
		if err != nil {
			initWarns = append(initWarns, err)
		} else {
			clock = pcf
		}
	} else {
		clock = ds
	}

	board.MCU.PetWatchdog()
	debug.Log("probing notecard...")

	// Blues Notecard
	nc, err := notecard.New(board.I2C.TxFn)
	if err != nil {
		initWarns = append(initWarns, err)
	}

	runtime.GC()
	board.MCU.PetWatchdog()
	debug.Log("probing sd...")

	// SD Card
	type sdEntry struct {
		card *sdcard.Card
		cs   hal.Pin
	}
	var cards []sdEntry
	for _, cs := range board.SDCSPins {
		board.MCU.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(board.SPI.Bus, board.SPI.SCK, board.SPI.SDO, board.SPI.SDI, cs)
		if err != nil {
			if len(cards) == 0 {
				initWarns = append(initWarns, errors.New("CS "+pin+": "+err.Error()))
			}
			continue
		}
		cards = append(cards, sdEntry{card: c, cs: cs})
	}

	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	board.MCU.PetWatchdog()

	// Config
	cfg := config.Default(board.LED.Pin(), board.Sensors)

	if nc != nil {
		if err := nc.Configure(&cfg); err != nil {
			initErrs = append(initErrs, err)
		}
		board.MCU.PetWatchdog()
	}

	if nc == nil && card != nil {
		raw, err := card.ReadFile("CONFIG.INI")
		if err != nil {
			// No CONFIG.INI on SD card; attempt to make one.
			if ini, mErr := cfg.Marshal(); mErr != nil {
				initErrs = append(initErrs, errors.New("config: "+mErr.Error()))
			} else if wErr := card.WriteFile("CONFIG.INI", ini); wErr != nil {
				initErrs = append(initErrs, errors.New("sd: "+wErr.Error()))
			}
		} else if raw != nil {
			// Found CONFIG.INI on SD card; attempt to parse it.
			if err := config.Parse(raw, &cfg); err != nil {
				if joined, ok := err.(interface{ Unwrap() []error }); ok {
					for _, e := range joined.Unwrap() {
						initErrs = append(initErrs, errors.New("config: "+e.Error()))
					}
				} else {
					initErrs = append(initErrs, errors.New("config: "+err.Error()))
				}
			}
		}
	}

	if cfg.Heartbeat.LedPin != hal.NoPin {
		board.LED = led.New(cfg.Heartbeat.LedPin)
	}

	// Log sinks
	serialSink := serial.NewSink(board.Serial)

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

	var sdCardFileSink *file.Sink
	if card != nil {
		if err := card.Mkdir("GLOOM"); err != nil {
			initErrs = append(initErrs, errors.New("sd: "+err.Error()))
		}

		opener := func(name string) (file.AppendFile, error) {
			return card.OpenAppend(name)
		}
		sdCardFileSink, err = file.New("sd", opener, file.FileSpec{
			Dir: "GLOOM",
			Ext: ".CSV",
		}, file.FileSpec{
			Dir: "GLOOM",
			Ext: ".LOG",
		}, now)
		if err != nil {
			initErrs = append(initErrs, errors.New("file: "+err.Error()))
		}
	}

	var notehubSink *notehub.Sink
	if nc != nil {
		notehubSink, err = notehub.New(nc)
		if err != nil {
			initErrs = append(initErrs, errors.New("notehub: "+err.Error()))
		}
	}

	board.MCU.PetWatchdog()

	// Logger and recorders
	logger := log.NewLogger(now)
	logger.AddSink(serialSink, log.LevelDebug)
	recorders := []sensor.Recorder{serialSink}
	if notehubSink != nil {
		logger.AddSink(notehubSink, cfg.Blues.LogLevel)
		recorders = append(recorders, notehubSink)
	}
	if sdCardFileSink != nil {
		logger.AddSink(sdCardFileSink, cfg.SD.LogLevel)
	}

	// Sensors
	sensorRegistry := make(map[string]sensor.Sensor)
	if board.ADCPin != hal.NoPin {
		sensorRegistry["vbat"] = vbat.NewDevice(board.ADCPin)
		board.MCU.PetWatchdog()
	}

	var sensors []sensor.Sensor
	for _, id := range cfg.Sample.Sensors {
		dev, ok := sensorRegistry[id]
		if ok {
			sensors = append(sensors, dev)
		}
	}

	board.LED.Off()

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

	p, err := cfg.Marshal()
	if err != nil {
		logger.Error("marshal: " + err.Error())
	} else {
		_, _ = debug.W.Write(p)
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
	sleeper := sleeper.New(board.MCU, clock, rails)
	man := manager.New(sleeper, cfg, sensors, recorders, logger)

	// Register sample's external interrupt pin with the sleeper.
	if cfg.Sample.ExtPin != hal.NoPin {
		sleeper.AddWakePin(cfg.Sample.ExtPin)
	}

	// Validate there is at least one wake source.
	if cfg.Sample.Interval <= 0 && cfg.Sample.ExtPin == hal.NoPin {
		err := errors.New("config: no wake sources configured (sample needs interval > 0 or ext_pin)")
		logger.Error(err.Error())
		fatal(err, board.LED)
	}

	if cfg.Heartbeat.Interval > 0 && cfg.Heartbeat.LedPin != hal.NoPin {
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
