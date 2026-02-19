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

Define the port to the usb serial adapter in your `.envrc` file. For example on macOS using the Adafruit FTDI Serial TTL-232 USB Cable:

```
export GLOOM_SERIAL_PORT=/dev/cu.usbserial-AI04YQAD
```

And run: 

```
make monitor
```

Output is automatically written to `debug.log`.
