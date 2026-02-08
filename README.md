# Gloom

A sleepy IoT firmware for low-power sensor sampling, written in [TinyGo](https://tinygo.org/).

Gloom runs on SAMD21 microcontrollers paired with the [Hypnos](https://github.com/OPEnSLab-OSU/OPEnS-Lab-Home/wiki/Hypnos) board from OPEnS Lab. It puts the system into standby mode (deep sleep) between sensor readings, waking on RTC alarm interrupts to sample sensors and log measurements to an SD card and/or MQTT server.
