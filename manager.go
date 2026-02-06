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

type Manager struct {
	sys    hardware.Platform
	config Config
	logger *log.Logger
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
			for !machine.Serial.DTR() {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	man.logger = log.NewLogger(man.config.LogLevel, man.config.SerialEnabled)
	for _, err := range initErrs {
		man.Log(log.LevelError, "init: "+err.Error())
	}
	man.Log(log.LevelInfo, "platform: "+man.sys.Name())

	return &man, nil
}

func (man *Manager) Sleep() hardware.WakeReason {
	man.Log(log.LevelDebug, "sleep: sample="+man.config.SampleInterval.String()+
		" heartbeat="+man.config.HeartbeatInterval.String())

	reason, err := man.sys.Sleep(man.config.SampleInterval, man.config.HeartbeatInterval)
	if err != nil {
		man.Log(log.LevelError, "sleep: "+err.Error())
	}

	return reason
}

func (man *Manager) Log(level log.Level, msg string) {
	t, err := man.sys.ReadTime()
	if err != nil {
		t = time.Now()
		man.logger.Log(t, log.LevelError, "rtc: "+err.Error())
	}

	man.logger.Log(t, level, msg)
}

func formatBytes(b uint64) string {
	whole := b / 1024
	return strconv.FormatUint(whole, 10)
}

func (man *Manager) LogMem() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	man.Log(log.LevelDebug, "mem: heap_alloc="+formatBytes(m.HeapAlloc)+"kb"+
		" heap_sys="+formatBytes(m.HeapSys)+"kb")
}
