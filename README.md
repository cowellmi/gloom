# Gloom

A sleepy IoT firmware for low-power sensor sampling, written in [TinyGo](https://tinygo.org/).

## Development

### Dev Environment

This project uses [Nix](https://nixos.org/) flakes for toolchain management and a `.env` file for project configuration.

```
cp .env.example .env
```

Edit `.env` to set `GLOOM_PORT` and other variables for your setup. The Makefile loads `.env` directly, so `make build`, `make flash`, etc. work out of the box.

If you use [direnv](https://direnv.net/), run `direnv allow` and it will automatically activate the Nix dev shell when you start a terminal session in the project directory. The `.envrc` bootstraps [nix-direnv](https://github.com/nix-community/nix-direnv) to cache the dev shell. Without direnv, activate the Nix shell manually with `nix develop`.

### Serial Monitor

| Serial Adapter (wire color) | Feather M0 Pin | Connection Logic |
| ------------- | ------------- | ------------- |
| GND (Black) | GND | Common ground
| RX (Yellow) | TX (D1) | Feather TX -> Adapter RX
| TX (Orange) | RX (D0) | Feather RX <- Adapter TX

#### When to use UART instead of USB CDC?

I recommend using a USB serial adapter to read UART when working with a system that has an RTC (e.g. Hypnos board). Any system with a supported RTC and an MCU with a low-power standby mode will enter a deep sleep after every wake cycle. USB CDC connection is unstable after waking from standby mode on the Feather M0. If you are not working with a Hypnos board, then the device will enter a busy-wait loop and the USB connection should be undisturbed.

#### How to connect serial monitor?

Define the port to the usb serial adapter in your `.env` file. For example on macOS using the Adafruit FTDI Serial TTL-232 USB Cable:

```
GLOOM_SERIAL_PORT=/dev/cu.usbserial-AI04YQAD
```

And run: 

```
make monitor
```

Output is automatically written to `debug.log`.
