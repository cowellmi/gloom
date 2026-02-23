//go:build tinygo

package main

import (
	"errors"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/config"
	"github.com/cowellmi/gloom/internal/debug"
	"github.com/cowellmi/gloom/internal/drivers/ds3231"
	"github.com/cowellmi/gloom/internal/drivers/pcf8523"
	"github.com/cowellmi/gloom/internal/hal"
	"github.com/cowellmi/gloom/internal/log"
	"github.com/cowellmi/gloom/internal/manager"
	"github.com/cowellmi/gloom/internal/sdcard"
	"github.com/cowellmi/gloom/internal/sensor"
	"github.com/cowellmi/gloom/internal/sensor/vbat"
	"github.com/cowellmi/gloom/internal/sink/file"
	"github.com/cowellmi/gloom/internal/sink/serial"
	"github.com/cowellmi/gloom/internal/sleeper"
	"github.com/cowellmi/gloom/internal/wait"
)

func main() {
	var initErrs []error
	var initWarns []error

	// --- Board ---
	board := initBoard()
	board.LED.On()
	board.MCU.PaintStack()
	board.MCU.EnableWatchdog()
	debug.W = board.Serial

	board.MCU.PetWatchdog()
	debug.Log("powering rails...")

	// --- Power rails ---
	rails := initRails()
	if rails != nil {
		rails.PowerOff()
		wait.For(250 * time.Millisecond)
		rails.PowerOn(false)
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

	// --- RTC ---
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

	now := time.Now()
	if clock != nil {
		t, err := clock.ReadTime()
		if err != nil {
			initErrs = append(initErrs, errors.New("rtc: "+err.Error()))
		} else {
			now = t
		}
	}

	board.MCU.PetWatchdog()
	debug.Log("probing sd...")

	// --- SD Card ---
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
			initWarns = append(initWarns, errors.New("CS "+pin+": "+err.Error()))
			continue
		}
		cards = append(cards, sdEntry{card: c, cs: cs})
	}

	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	board.MCU.PetWatchdog()

	// --- Sensors ---
	sensorRegistry := make(map[string]func() sensor.Device)

	if board.ADCPin != 0 {
		sensorRegistry["vbat"] = func() sensor.Device {
			return vbat.NewDevice(board.ADCPin)
		}
		board.MCU.PetWatchdog()
	}

	board.MCU.PetWatchdog()

	// --- Config ---
	cfg := config.Default()

	if card != nil {
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

	board.MCU.PetWatchdog()

	// --- Log sinks ---
	serialSink := serial.NewSink(board.Serial)

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

	// --- Logger ---
	logger := log.NewLogger(now)

	for _, ls := range cfg.Device.LogSinks {
		switch ls.Name {
		case "serial":
			logger.AddSink(serialSink, ls.Level)
		case "sd":
			if sdCardFileSink != nil {
				logger.AddSink(sdCardFileSink, ls.Level)
			} else {
				initWarns = append(initWarns, errors.New(""))
			}
		}
	}

	board.MCU.PetWatchdog()

	// --- Sensor data sinks ---

	var recorders []sensor.Recorder
	for _, name := range cfg.Device.DataSinks {
		switch name {
		case "serial":
			recorders = append(recorders, serialSink)
		case "sd":
			if sdCardFileSink != nil {
				recorders = append(recorders, sdCardFileSink)
			}
		}
	}

	// --- Resolve groups ---

	sensorPool := make(map[string]sensor.Device)
	var groups []manager.Group
	for _, gcfg := range cfg.Groups {
		g := manager.Group{
			Name:     gcfg.Name,
			Interval: gcfg.Interval,
			PulseLED: gcfg.PulseLED,
			Host:     gcfg.Host,
			Payload:  gcfg.Payload,
		}

		for _, id := range gcfg.Sensors {
			dev, ok := sensorPool[id]
			if !ok {
				newDevice, found := sensorRegistry[id]
				if !found {
					initErrs = append(initErrs, errors.New("config: ["+gcfg.Name+"] Unknown sensor: "+id))
					continue
				}
				dev = newDevice()
				sensorPool[id] = dev
				board.MCU.PetWatchdog()
			}
			g.Sensors = append(g.Sensors, dev)
		}

		groups = append(groups, g)
	}

	board.LED.Off()

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
		logger.Debug("rtc: NONE")
	}

	if len(cards) > 0 {
		sd := "sd:"
		for _, e := range cards {
			sd += " CS " + strconv.Itoa(int(e.cs))
		}
		logger.Debug(sd)
		if !needsSDSink(&cfg) {
			logger.Warn("sd: Card detected but not configured as a sink.")
		}
	} else {
		logger.Debug("sd: NONE")
	}

	if len(cfg.Device.LogSinks) > 0 {
		b := []byte("log sinks:")
		for _, ls := range cfg.Device.LogSinks {
			b = append(b, ' ')
			b = append(b, ls.Name...)
		}
		logger.Debug(string(b))
	}

	if len(cfg.Device.DataSinks) > 0 {
		b := []byte("data sinks:")
		for _, ds := range cfg.Device.DataSinks {
			b = append(b, ' ')
			b = append(b, ds...)
		}
		logger.Debug(string(b))
	}

	if len(cfg.Groups) > 0 {
		b := []byte("groups:")
		for _, g := range cfg.Groups {
			b = append(b, ' ')
			b = append(b, g.Name...)
		}
		logger.Debug(string(b))
	} else {
		logger.Debug("groups: none")
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b := []byte("mem: heap_sys=")
	b = strconv.AppendUint(b, ms.HeapSys/1024, 10)
	b = append(b, "kb"...)
	if ss := board.MCU.StackSize(); ss > 0 {
		b = append(b, " stack_size="...)
		b = strconv.AppendUint(b, uint64(ss/1024), 10)
		b = append(b, "kb"...)
	}
	logger.Debug(string(b))

	if clock == nil {
		for _, g := range cfg.Groups {
			if g.Interval > 0 {
				logger.Warn("config: timed groups configured without an RTC")
				logger.Warn("config: deep sleep disabled; using idle sleep")
				break
			}
		}
	}

	board.MCU.PetWatchdog()

	// --- System ---

	hypnos := sleeper.New(board.MCU, clock, rails)

	// --- Register external interrupt pins ---

	man := manager.New(hypnos, groups, recorders, logger)
	for i, g := range cfg.Groups {
		if g.ExternalIntPin > 0 {
			pin := hal.Pin(g.ExternalIntPin)
			hypnos.AddWakePin(pin)
			man.RegisterExternalPin(uint8(pin), i)
		}
	}

	man.SetLED(board.LED)
	man.EnableWatchdog(board.MCU.PetWatchdog)
	man.SetStackMonitor(board.MCU.StackUsed)

	man.Run()
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
		led.On()
		wait.For(250 * time.Millisecond)
		led.Off()
		wait.For(250 * time.Millisecond)
	}
}
