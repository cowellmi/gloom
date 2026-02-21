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

	cfg := config.Default()

	board := initBoard(&cfg)
	board.MCU.ConfigureLED(cfg.Device.LedPin)
	board.MCU.LedOn()
	board.MCU.EnableWatchdog()

	debug.W = board.UART
	debug.Log("loading defaults...")

	// TODO: probe Blues Notecard on I2C 0x17.

	board.MCU.PetWatchdog()
	debug.Log("powering rails...")

	// --- Power rails ---
	var rails hal.Rails
	if r := boardPower(); len(r) > 0 {
		rails = power.NewController(r...)
	}

	board.MCU.PetWatchdog()
	debug.Log("configuring I2C...")

	if err := board.MCU.ConfigureI2C(board.SDA, board.SCL); err != nil {
		board.MCU.DisableWatchdog()
		fatal(err, board.MCU)
	}

	board.MCU.PetWatchdog()
	debug.Log("probing rtc...")

	// --- RTC probe ---
	var clock hal.RTC
	ds, err := ds3231.Probe(board.I2C, cfg.Device.RTCWakePin)
	if err != nil {
		board.MCU.PetWatchdog()
		initWarns = append(initWarns, err)
		pcf, err := pcf8523.Probe(board.I2C, cfg.Device.RTCWakePin)
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

	// --- SD probe ---
	type sdEntry struct {
		card *sdcard.Card
		cs   uint8
	}
	var cards []sdEntry
	for _, cs := range cfg.Device.SDCSPins {
		board.MCU.PetWatchdog()
		pin := strconv.Itoa(int(cs))
		c, err := sdcard.NewCard(board.SPI.Bus, board.SPI.SCK, board.SPI.SDO, board.SPI.SDI, cs)
		if err != nil {
			initWarns = append(initWarns, errors.New("CS: "+pin+": "+err.Error()))
			continue
		}
		cards = append(cards, sdEntry{card: c, cs: cs})
	}

	var card *sdcard.Card
	if len(cards) > 0 {
		card = cards[0].card
	}

	board.MCU.PetWatchdog()

	// --- Config loading ---
	if card != nil {
		raw, err := card.ReadFile("CONFIG.INI")
		if err != nil {
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

	board.MCU.ConfigureLED(cfg.Device.LedPin)

	board.MCU.PetWatchdog()

	// --- Log sinks ---

	uartSink := serial.NewSink(board.UART)
	usbSink := serial.NewSink(board.USBCDC)

	var fileSink *file.Sink
	if card != nil {
		if err := card.Mkdir("DATA"); err != nil {
			initErrs = append(initErrs, err)
		}
		if err := card.Mkdir("LOGS"); err != nil {
			initErrs = append(initErrs, err)
		}

		now := time.Now()
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
		}
	}

	board.MCU.PetWatchdog()

	// --- Logger ---

	now := time.Now()
	if clock != nil {
		if t, err := clock.ReadTime(); err == nil {
			now = t
		}
	}
	logger := log.NewLogger(now)

	for _, ls := range cfg.Device.LogSinks {
		switch ls.Name {
		case "uart":
			logger.AddSink(uartSink, ls.Level)
		case "usb":
			logger.AddSink(usbSink, ls.Level)
		case "sd":
			if fileSink != nil {
				logger.AddSink(fileSink, ls.Level)
			}
		}
	}

	board.MCU.PetWatchdog()

	// --- Sensor data sinks ---

	var recorders []sensor.Recorder
	for _, name := range cfg.Device.DataSinks {
		switch name {
		case "uart":
			recorders = append(recorders, uartSink)
		case "usb":
			recorders = append(recorders, usbSink)
		case "sd":
			if fileSink != nil {
				recorders = append(recorders, fileSink)
			}
		case "blues":
			// TODO: Blues Notecard sink
		default:
			initErrs = append(initErrs, errors.New("unknown data_sink: "+name))
		}
	}

	// --- Resolve groups ---

	sensorPool := make(map[string]sensor.Device)
	var groups []manager.Group
	for _, gcfg := range cfg.Groups {
		g := manager.Group{
			Name:     gcfg.Name,
			PulseLED: gcfg.PulseLED,
			Host:     gcfg.Host,
			Payload:  gcfg.Payload,
		}

		for _, id := range gcfg.Sensors {
			dev, ok := sensorPool[id]
			if !ok {
				newDevice, found := sensorRegistry[id]
				if !found {
					initErrs = append(initErrs, errors.New("["+gcfg.Name+"] unknown sensor: "+id))
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

	board.MCU.LedOff()

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

	board.MCU.PetWatchdog()

	// --- System ---

	intervals := make([]time.Duration, len(cfg.Groups))
	for i, g := range cfg.Groups {
		intervals[i] = g.Interval
	}
	sys := hal.NewSystem(board.MCU, clock, rails, intervals)

	for i, g := range cfg.Groups {
		if g.ExternalIntPin > 0 {
			sys.RegisterExternalPin(g.ExternalIntPin, i)
		}
	}

	// --- Manager ---

	man := manager.New(sys, groups, recorders, logger)
	man.SetLED(board.MCU.LedOn, board.MCU.LedOff)
	man.EnableWatchdog(board.MCU.PetWatchdog)

	man.Run()
}

// fatal blinks the LED forever to signal a hard failure when no
// serial monitor is connected.
func fatal(err error, mcu hal.MCU) {
	debug.Log("FATAL: " + err.Error())
	for {
		mcu.LedOn()
		wait.For(250 * time.Millisecond)
		mcu.LedOff()
		wait.For(250 * time.Millisecond)
	}
}
