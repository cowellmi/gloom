package main

import (
	"machine"
	"runtime"
	"strconv"
	"time"

	"github.com/cowellmi/gloom/internal/hardware"
	"github.com/cowellmi/gloom/internal/hardware/fallback"
	"github.com/cowellmi/gloom/internal/hardware/hypnos"
	"github.com/cowellmi/gloom/internal/log"
)

const (
	maxWaitForSerialDTR = 1200
)

type Manager struct {
	sys      hardware.Platform
	config   Config
	logger   *log.Logger
	wakeTime time.Time
}

func NewManager() (*Manager, error) {
	var man Manager
	var err error
	var initErrs []error // Keep track of init errors until logger is setup

	// Setup I2C
	err = machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 100e3, // 100 kHz
	})
	if err != nil {
		return nil, err
	}

	// Probe for platforms
	man.sys, err = hypnos.Probe(machine.I2C0)
	if err != nil {
		initErrs = append(initErrs, err)
		man.sys = fallback.New()
	}

	// Parse config
	man.config = DefaultConfig()
	data, err := man.sys.ReadFile("config.txt")
	if err != nil {
		initErrs = append(initErrs, err)
	} else {
		err = ParseConfig(data, &man.config)
		if err != nil {
			initErrs = append(initErrs, err)
		}
	}

	// Serial configuration
	if man.config.SerialEnabled {
		err = machine.Serial.Configure(machine.UARTConfig{
			BaudRate: 115200,
		})
		if err != nil {
			initErrs = append(initErrs, err)
		}

		if man.config.WaitForSerial {
			attempt := 0
			for !machine.Serial.DTR() {
				attempt += 1
				if attempt > maxWaitForSerialDTR {
					break
				}

				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	man.logger = log.NewLogger(man.config.LogLevel, man.config.SerialEnabled)
	for _, err := range initErrs {
		man.slog(log.LevelError, "init: "+err.Error())
	}

	return &man, nil
}

func (man *Manager) Run() {
	for {
		reason := man.sleep()

		if reason&hardware.WakeSample != 0 {
			man.sample()
		}
		if reason&hardware.WakeHeartbeat != 0 {
			man.heartbeat()
		}

		man.slog(log.LevelInfo, "platform: "+man.sys.Name())
		man.logMem()
	}
}

func (man *Manager) sleep() hardware.WakeReason {
	// Log intervals
	sampleInterval := man.config.SampleInterval.String()
	if man.config.SampleInterval == 0 {
		sampleInterval = "disabled"
	}
	heartbeatInterval := man.config.HeartbeatInterval.String()
	if man.config.SampleInterval == 0 {
		heartbeatInterval = "disabled"
	}
	man.slog(log.LevelDebug, "sleep: sample="+sampleInterval+" heartbeat="+heartbeatInterval)

	// Put system to sleep. Execution halts here until wake from sleep.
	reason, err := man.sys.Sleep(man.config.SampleInterval, man.config.HeartbeatInterval)
	if err != nil {
		man.slog(log.LevelError, "sleep: "+err.Error())
	}
	// Resume execution after wake from sleep.

	// Update wake up time.
	t, err := man.sys.ReadTime()
	if err != nil {
		t = time.Now()
		man.logger.Log(t, log.LevelError, "rtc: "+err.Error())
	}
	man.wakeTime = t

	return reason
}

func (man *Manager) sample() {
	for _, s := range man.config.Sensors {
		if err := s.Init(); err != nil {
			man.slog(log.LevelError, "failed to initialize: "+s.Name()+": "+err.Error())
			continue
		}

		ms, err := s.Measure()
		if err != nil {
			man.slog(log.LevelError, "failed to measure: "+s.Name()+": "+err.Error())
			continue
		}

		for _, m := range ms {
			man.slog(log.LevelInfo, s.Name()+": "+m.Label+": "+m.Value+" "+m.Unit)
		}
	}
}

func (man *Manager) heartbeat() {
	man.slog(log.LevelDebug, "heartbeat")
	// TODO: transmit keep-alive message
}

func (man *Manager) slog(level log.Level, msg string) {
	man.logger.Log(man.wakeTime, level, msg)
}

func formatBytes(b uint64) string {
	whole := b / 1024
	return strconv.FormatUint(whole, 10)
}

func (man *Manager) logMem() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	man.slog(log.LevelDebug, "mem: heap_alloc="+formatBytes(m.HeapAlloc)+"kb"+
		" heap_sys="+formatBytes(m.HeapSys)+"kb")
}
