# Gloom

A sleepy IoT firmware for low-power sensor sampling, written in [TinyGo](https://tinygo.org/).

## Notes

### Connecting USB <-> Serial Adapter

TinyGo's Feather M0 board file only exposes UART1 on SERCOM1 (D10/D11), but those pins are needed for the Hypnos SD card chip-select. The firmware manually configures UART0 on SERCOM0 using the D0/D1 header pins instead (see `cmd/gloom/main_feather_m0.go`). In the future, hopefully upstream TinyGo exports UART0.

| Serial Adapter (wire color) | Feather M0 Pin | Connection Logic |
| ------------- | ------------- | ------------- |
| GND (Black) | GND | Common ground
| RX (Yellow) | TX (D1) | Feather TX -> Adapter RX
| TX (Orange) | RX (D0) | Feather RX <- Adapter TX

#### Why use UART instead of USB Serial?

When we put the device into low-power standby mode, we are also closing the usb serial connection to save power. By using UART, we achieve a reliable serial connection without complex logic to ensure USB serial connection during each wake cycle.
