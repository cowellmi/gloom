//go:build tinygo

package main

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/fallback"
	"github.com/cowellmi/gloom/internal/fmtbuf"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/led"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/notecard"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/vbat"
	"github.com/cowellmi/gloom/internal/sink"
	"github.com/cowellmi/gloom/internal/sleeper"
	"github.com/cowellmi/gloom/internal/wait"
)

var ProductUID string
var buf [128]byte

func logMem(stackUsed func() uint) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := buf[:0]
	b = fmtbuf.Append(b, "mem: heap_alloc=")
	b = fmtbuf.AppendUint(b, ms.HeapAlloc, 10)
	b = fmtbuf.AppendByte(b, 'B')
	if stackUsed != nil {
		b = fmtbuf.Append(b, " stack_alloc=")
		b = fmtbuf.AppendUint(b, uint64(stackUsed()), 10)
		b = fmtbuf.AppendByte(b, 'B')
	}
	debug.Log(string(b))
}

func main() {
	var initErrs []error
	var initWarns []error

	board := initBoard()
	board.MCU.PaintStack()
	board.MCU.EnableWatchdog()

	debug.W = board.Serial

	wing := initWing()
	board.MCU.PetWatchdog()

	var statusLED hal.LED = fallback.LED{}
	if board.LEDPin != hal.NoPin {
		statusLED = led.New(board.LEDPin)
		statusLED.On()
	}

	for i := 0; i < 5; i++ {
		debug.W.Write([]byte("."))
		wait.For(time.Second)
		board.MCU.PetWatchdog()
	}
	debug.W.Write([]byte("\r\n"))

	err := board.MCU.ConfigureI2C(board.SDA, board.SCL)
	if err != nil {
		panic(err)
	}
	board.MCU.PetWatchdog()

	logMem(board.MCU.StackUsed)

	// Probe RTC; only use its interrupt pin if the hardware is present.
	rtcIntPin := hal.NoPin
	rtc, err := wing.ProbeRTC(board.I2C)
	if err != nil {
		initWarns = append(initWarns, err)
		rtc = fallback.Clock{}
	} else {
		rtcIntPin = wing.RTCInterruptPin
	}

	logMem(board.MCU.StackUsed)

	nc, ncErr := notecard.New(board.I2C.Tx)
	if ncErr != nil {
		initWarns = append(initWarns, ncErr)
	}
	if nc != nil {
		nc.SetWatchdog(board.MCU.PetWatchdog)
	}
	board.MCU.PetWatchdog()
	runtime.GC()

	logMem(board.MCU.StackUsed)

	cfg := config.Default(board.LEDPin, rtcIntPin, board.Sensors, wing.SDChipSelectPins)
	if nc != nil {
		board.MCU.PetWatchdog()
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
			// Pre-serialize the body before calling RequestResponse so that the
			// deep marshalMap recursion over the nested Config map completes and
			// unwinds before the outer request encoding begins. Passing the nested
			// map directly would combine both recursion chains on the stack and
			// overflow the goroutine's fixed-size stack.
			bodyJSON := notecard.Marshal(cfg.MarshalMap())
			board.MCU.PetWatchdog()
			if _, wErr := nc.RequestResponse(map[string]any{
				"req":  "note.add",
				"file": "config.db",
				"note": "config",
				"body": notecard.RawJSON(bodyJSON),
				"sync": true,
			}); wErr != nil {
				initErrs = append(initErrs, errors.New("notecard: config.db: "+wErr.Error()))
			}
		default:
			initErrs = append(initErrs, errors.New("notecard: config.db: "+rErr.Error()))
		}
	}
	board.MCU.PetWatchdog()

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
				initWarns = append(initWarns, errors.New("sd cs "+pin+": "+err.Error()))
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
		raw, err := card.ReadFile("CONFIG.JSON")
		if err != nil {
			// TODO: check for specific file not found err
			debug.Log("writing default config to SD card...")
			if js, mErr := cfg.MarshalJSON(); mErr != nil {
				initErrs = append(initErrs, errors.New("config: "+mErr.Error()))
			} else if wErr := card.WriteFile("CONFIG.JSON", js); wErr != nil {
				initErrs = append(initErrs, errors.New("sd: "+wErr.Error()))
			}
		} else {
			debug.Log("loading config from SD card...")
			if pErr := config.ParseJSON(raw, &cfg); pErr != nil {
				initErrs = append(initErrs, errors.New("config: "+pErr.Error()))
			}
		}
	}

	if cfg.LEDPin != hal.NoPin {
		statusLED = led.New(cfg.LEDPin)
	}

	now, err := rtc.ReadTime()
	if err != nil {
		initErrs = append(initErrs, err)
		rtc = fallback.Clock{}
	}

	logger := log.NewLogger(now)

	var dataSinks []sink.DataSink

	// Serial: always available (hardware always present).
	if sc, ok := cfg.Sinks["serial"]; ok {
		serialSink := sink.NewSerial(board.Serial)
		if sc.LogLevel < config.LogLevelOff {
			logger.AddSink(serialSink, sc.LogLevel)
		}
		dataSinks = append(dataSinks, serialSink)
	}

	// Blues: only if Notecard is present.
	if nc != nil {
		if sc, ok := cfg.Sinks["blues"]; ok {
			notehubSink := sink.NewNotehubSink(nc, "data.qo", "log.qo")
			if sc.LogLevel < config.LogLevelOff {
				logger.AddSink(notehubSink, sc.LogLevel)
			}
			dataSinks = append(dataSinks, notehubSink)
		}
		board.MCU.PetWatchdog()
	}

	// SD: only if card is present.
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
			if sc, ok := cfg.Sinks["sd"]; ok {
				if sc.LogLevel < config.LogLevelOff {
					logger.AddSink(sdCardFileSink, sc.LogLevel)
				}
				dataSinks = append(dataSinks, sdCardFileSink)
			}
		}
		board.MCU.PetWatchdog()
	}

	defer func() {
		if r := recover(); r != nil {
			board.MCU.DisableWatchdog()
			msg := "PANICKED: "
			switch v := r.(type) {
			case error:
				msg += v.Error()
			case string:
				msg += v
			}
			logger.Log(config.LogLevelError, msg)
		}
	}()

	for _, err := range initWarns {
		logger.LogError(config.LogLevelWarn, err, "init: ")
	}
	for _, err := range initErrs {
		logger.LogError(config.LogLevelError, err, "init: ")
	}

	// Build shared sensor registry (one instance per sensor ID).
	sensors := map[string]sensor.Sensor{}
	if board.ADCPin != hal.NoPin {
		sensors["vbat"] = vbat.NewDevice(board.ADCPin)
	}

	// Warn about sensor IDs referenced in config but not in registry.
	for _, g := range cfg.Groups {
		for _, id := range g.Sensors {
			if _, ok := sensors[id]; !ok {
				logger.Log(config.LogLevelWarn, "sensors: unknown id: "+id)
			}
		}
		board.MCU.PetWatchdog()
	}

	statusLED.Off()
	logger.Log(config.LogLevelInfo, "mcu: "+board.MCU.Identifier())
	logger.Log(config.LogLevelInfo, "rtc: "+rtc.Identifier())
	logger.Log(config.LogLevelInfo, "rails: "+wing.Rails.Identifier())

	if len(cards) > 0 {
		sd := "sd: cs"
		for _, e := range cards {
			sd += " " + strconv.Itoa(int(e.cs))
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

	// Collect all unique interrupt pins: RTC pin (from config) + per-group pins.
	var allPins []hal.Pin
	if cfg.RTCIntPin != hal.NoPin {
		allPins = append(allPins, cfg.RTCIntPin)
	}
	for _, g := range cfg.Groups {
		for _, pin := range g.InterruptPins {
			found := false
			for _, p := range allPins {
				if p == pin {
					found = true
					break
				}
			}
			if !found {
				allPins = append(allPins, pin)
			}
		}
	}

	// Manager
	slp := sleeper.New(board.MCU, rtc, wing.Rails, board.SDA, board.SCL, allPins)

	// Validate that at least one wake source is configured.
	if err := config.Validate(&cfg); err != nil {
		logger.LogError(config.LogLevelError, err, "")
		panic(err)
	}

	man := manager.New(slp, wing.Rails, statusLED, cfg.Groups, sensors, dataSinks, logger)
	man.EnableWatchdog(board.MCU.PetWatchdog)
	man.SetStackMonitor(board.MCU.StackUsed)

	now, err = rtc.ReadTime()
	if err != nil {
		rtc = fallback.Clock{}
		logger.LogError(config.LogLevelError, err, "rtc: ")
	}
	man.Run(now)
}
