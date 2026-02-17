# Gloom

A sleepy IoT firmware for low-power sensor sampling, written in [TinyGo](https://tinygo.org/).

## Development

### Serial Monitor

| Serial Adapter (wire color) | Feather M0 Pin | Connection Logic |
| ------------- | ------------- | ------------- |
| GND (Black) | GND | Common ground
| RX (Yellow) | TX (D1) | Feather TX -> Adapter RX
| TX (Orange) | RX (D0) | Feather RX <- Adapter TX

#### When to use UART instead of USB CDC?

I recommend using a USB serial adapter to read UART when working with a system that an RTC (e.g. Hypnos board). Any system with a supported RTC and an MMU with a low-power standby mode will enter a deep sleep after every wake cycle. USB CDC connection is unstable after waking from standby mode on the Feather M0. If you are not working with a Hypnos board, then the device will enter a busy-wait loop and the USB connection should be undisturbed.

#### How to connect serial monitor?

Replace with your device:

```
just monitor /dev/DEVICE_NAME
```

You can also define the device in your .envrc file:

```
export GLOOMPORT=/dev/ttyACM0
```

And run: 

```
just monitor
```
