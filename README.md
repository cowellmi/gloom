# Gloom

A sleepy IoT firmware for low-power sensor sampling, written in [TinyGo](https://tinygo.org/).

## Notes

### Connecting USB <-> Serial Adapter

For some reason the TinyGo `machine` code for Feather M0 doesn't include a definition of UART0 (which would be the TX and RX pins on the Feather M0). But, it does include the definition for UART1 which has the following default pin configuration:

```
UART_TX_PIN = D10
UART_RX_PIN = D11
```

Which correspond to the pins labeled 10 and 11 on the Feather M0 (labeled 10 and CS on the Hypnos).

| Serial Adapter (wire color) | Feather M0 Pin | Connection Logic |
| ------------- | ------------- | ------------- |
| GND (Black) | GND | Common ground
| RX (Yellow) | 10 | Feather TX -> Adapter RX
| TX (Orange) | 11 | Feather RX <- Adapter TX


#### Why use UART instead of USB Serial?

When we put the device into low-power standby mode, we are also closing the usb serial connection to save power. By using UART, we achieve a reliable serial connection without complex logic to ensure USB serial connection during each wake cycle. *It just works*:tm:.
