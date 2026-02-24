//go:build tinygo

package main

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	tinynote "github.com/blues/note-tinygo"
	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/led"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/rtc/ds3231"
	"github.com/cowellmi/gloom/internal/rtc/pcf8523"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/vbat"
	"github.com/cowellmi/gloom/internal/sink/file"
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

	// Sensors
	sensorRegistry := make(map[string]func() sensor.Sensor)

	if board.ADCPin != hal.NoPin {
		sensorRegistry["vbat"] = func() sensor.Sensor {
			return vbat.NewDevice(board.ADCPin)
		}
		board.MCU.PetWatchdog()
	}

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
	debug.Log("probing notecard...")

	// Blues Notecard
	var hasNotecard bool
	var notecardUID string
	notecard, err := tinynote.OpenI2C(tinynote.DefaultI2CAddress, board.I2C.TxFn)
	if err != nil {
		initWarns = append(initWarns, err)
	} else if notecard != nil {
		req := tinynote.NewRequest("card.version")
		res, err := notecard.RequestResponse(req)
		if tinynote.IsError(err, res) {
			err = errors.New(tinynote.ErrorString(err, res))
			initErrs = append(initErrs, err)
		} else {
			hasNotecard = true
			notecardUID, _ = res["device"].(string)
		}
		board.MCU.PetWatchdog()
	}

	// Config
	cfg := config.Default()

	if hasNotecard {
		// Send env.template every boot — idempotent, defines expected
		// env var keys and their types for the Notehub UI.
		tmplReq := tinynote.NewRequest("env.template")
		tmplBody := tinynote.NewBody()
		tmplBody["log_sinks"] = "a"
		tmplBody["data_sinks"] = "a"
		tmplBody["led_pin"] = 21
		tmplBody["interval"] = "a"
		tmplBody["sensors"] = "a"
		tmplBody["ext_pin"] = 21
		tmplBody["heartbeat"] = "a"
		tmplBody["payload"] = "a"
		tmplBody["blink_led"] = true
		tmplReq["body"] = tmplBody
		notecard.Request(tmplReq)
		board.MCU.PetWatchdog()

		// Fetch env vars and apply to config.
		req := tinynote.NewRequest("env.get")
		res, err := notecard.RequestResponse(req)
		board.MCU.PetWatchdog()
		if err == nil && !tinynote.IsError(err, res) {
			if body, ok := res["body"].(map[string]interface{}); ok {
				if err := config.ParseMap(&cfg, body); err != nil {
					initWarns = append(initWarns, errors.New("env: "+err.Error()))
				}
			}
		}
	}

	if !hasNotecard && card != nil {
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

	if cfg.Device.LedPin != hal.NoPin {
		board.LED = led.New(cfg.Device.LedPin)
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
	if card != nil && needsSDSink(&cfg) {
		if err := card.Mkdir("GLOOM"); err != nil {
			initErrs = append(initErrs, errors.New("sd: "+err.Error()))
		}

		opener := func(name string) (file.AppendFile, error) {
			return card.OpenAppend(name)
		}
		var fileErr error
		sdCardFileSink, fileErr = file.New("sd", opener, file.FileSpec{
			Dir: "GLOOM",
			Ext: ".CSV",
		}, file.FileSpec{
			Dir: "GLOOM",
			Ext: ".LOG",
		}, now)
		if fileErr != nil {
			initErrs = append(initErrs, errors.New("sd: "+fileErr.Error()))
		}
	}

	board.MCU.PetWatchdog()

	// Logger
	logger := log.NewLogger(now)

	logger.AddSink(serialSink, log.LevelDebug)

	for _, ls := range cfg.Device.LogSinks {
		switch ls.Name {
		case "sd":
			if sdCardFileSink != nil {
				logger.AddSink(sdCardFileSink, ls.Level)
			} else {
				initWarns = append(initWarns, errors.New("log sink 'sd' configured but no SD card"))
			}
		}
	}

	board.MCU.PetWatchdog()

	// Sensor data sinks
	var recorders []sensor.Recorder
	recorders = append(recorders, serialSink)
	for _, name := range cfg.Device.DataSinks {
		switch name {
		case "sd":
			if sdCardFileSink != nil {
				recorders = append(recorders, sdCardFileSink)
			}
		}
	}

	// Resolve sensor IDs from config into sensor instances for the manager.

	sensorPool := make(map[string]sensor.Sensor)
	var sensors []sensor.Sensor
	for _, id := range cfg.Sample.Sensors {
		dev, ok := sensorPool[id]
		if !ok {
			newDevice, found := sensorRegistry[id]
			if !found {
				initErrs = append(initErrs, errors.New("config: unknown sensor: "+id))
				continue
			}
			dev = newDevice()
			sensorPool[id] = dev
			board.MCU.PetWatchdog()
		}
		sensors = append(sensors, dev)
	}

	board.LED.Off()

	// Report init warnings and errors
	for _, w := range initWarns {
		logger.Warn("init: " + w.Error())
	}
	for _, e := range initErrs {
		logger.Error("init: " + e.Error())
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
		if !needsSDSink(&cfg) {
			logger.Warn("sd: card detected but not configured as a sink.")
		}
	} else {
		logger.Debug("sd: NONE")
	}

	if hasNotecard {
		logger.Debug("notecard: " + notecardUID)
	} else {
		logger.Debug("notecard: NONE")
	}

	var bootBuf [256]byte

	if len(cfg.Device.LogSinks) > 0 {
		b := bootBuf[:0]
		b = fmtbuf.Append(b, "log sinks:")
		for _, ls := range cfg.Device.LogSinks {
			b = fmtbuf.AppendByte(b, ' ')
			b = fmtbuf.Append(b, ls.Name)
		}
		logger.Debug(string(b))
	}

	if len(cfg.Device.DataSinks) > 0 {
		b := bootBuf[:0]
		b = fmtbuf.Append(b, "data sinks:")
		for _, ds := range cfg.Device.DataSinks {
			b = fmtbuf.AppendByte(b, ' ')
			b = fmtbuf.Append(b, ds)
		}
		logger.Debug(string(b))
	}

	{
		b := bootBuf[:0]
		b = fmtbuf.Append(b, "sample: interval=")
		b = fmtbuf.Append(b, cfg.Sample.Interval.String())
		if cfg.Sample.ExtPin != hal.NoPin {
			b = fmtbuf.Append(b, " ext_pin=")
			b = fmtbuf.AppendUint(b, uint64(cfg.Sample.ExtPin), 10)
		}
		logger.Debug(string(b))
	}

	if cfg.Heartbeat.Interval > 0 {
		b := bootBuf[:0]
		b = fmtbuf.Append(b, "heartbeat: interval=")
		b = fmtbuf.Append(b, cfg.Heartbeat.Interval.String())
		logger.Debug(string(b))
	}

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

	if cfg.Heartbeat.Interval > 0 && cfg.Heartbeat.BlinkLED {
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

func needsSDSink(cfg *config.Config) bool {
	for _, ls := range cfg.Device.LogSinks {
		if ls.Name == "sd" {
			return true
		}
	}
	for _, ds := range cfg.Device.DataSinks {
		if ds == "sd" {
			return true
		}
	}
	return false
}

// fatal blinks the LED forever to signal a hard failure when no
// serial monitor is connected.
func fatal(err error, led hal.LED) {
	debug.Log("FATAL: " + err.Error())
	for {
		led.Blink()
	}
}
