# Hypnos Notes

According to the datasheet the Mode Entry for STANDBY mode is:

```
SCR.SLEEPDEEP = 1
WFI
```
> Before entering standby mode the user must make sure that a significant amount of clocks and peripherals are disabled, so that the voltage regulator is not overloaded
(Atmel-42181G–SAM-D21_Datasheet–09/2015 p. 124)

## Happy Path

First I am going to go through `libraries/Loom/examples/Hypnos/Hypnos_Sleep/Hypnos_Sleep.ino` and outline the steps required to put the SAMD21 in STANDBY mode.

### Setup

Do these steps once like during Arduino setup().

Configure pins for Hypnos power rails and on-board LED,

| Pin   | Component | Mode   |
| ----- | --------- | ------ |
| 5     | 3.3v rail | OUTPUT |
| 6     | 5v rail   | OUTPUT |
| 13    | Board LED | OUTPUT |


Turn on power rails and LED.

| Pin   | Component | Level  |
| ----- | --------- | ------ |
| 5     | 3.3v rail | LOW    | <-- active-low
| 6     | 5v rail   | HIGH   |
| 13    | Board LED | HIGH   |


Establish serial connection (for debugging purposes).


*This is where SD card would be initialized using SPI... but were going to save that for later.*


Configure I2C. Initialize DS3231. Clear alarm 1 and alarm 2.


Set the DS3231 SQW pin mode to OFF.


### Loop

Do these steps every time waking up (and during initial setup) like Arduino loop().


ISR = detach interrupt on pin 12.
Attach interrupt:
**Pin   Callback    Mode**
12      ISR         LOW

Configure generic clock. See: libraries/ArduinoLowPower-master/src/samd/ArduinoLowPower.cpp

Set alarm 1 based on current time and sample interval.

Power down all the sensors.

    Loom_Hypnos::pre_sleep()

Delay 1 second.

Close serial connection.

Detach USB.

Attach interrupt.

Turn off power rails.

    void ArduinoLowPowerClass::sleep()

Enter standby mode. System hangs here until interrupt.

WAKEUP!

Enable watchdog. Pet dog.

Attach USB. And then pet dog again...

Begin serial. Pet dog.

Turn on power rails. Pet dog.

Delay 1 second. Pet dog.

Clear alarm 1 and alarm 2. Pet dog.

Power up sensors. Need some way of petting/disabling the watchdog depending on the sensors e.g. LTE should disable the watchdog.

Wait for serial.
