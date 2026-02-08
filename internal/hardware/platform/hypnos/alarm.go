package hypnos

import (
	"machine"

	"github.com/cowellmi/gloom/internal/hardware/samd"
)

// clearAlarmInterrupt tears down the RTC alarm interrupt registration.
// Safe to call whether or not an interrupt is currently registered.
func clearAlarmInterrupt() {
	_ = AlarmPin.SetInterrupt(0, nil)
	samd.DisableWakeOnInterrupt(AlarmPin)
}

func alarmISR(p machine.Pin) {
	clearAlarmInterrupt()
}
