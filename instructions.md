# Connecting Raspberry Pi to Feather M0

## Wiring

Both devices run at 3.3V logic. USB provides power (5V) and common ground.

| RPi Pin        | Feather M0 Pin  | Purpose             |
| -------------  | --------------- | ------------------- |
| GPIO14 (TXD)   | RX (pin 0)      | RPi TX → Feather RX |
| GPIO15 (RXD)   | TX (pin 1)      | Feather TX → RPi RX |
| GPIO24         | SWDIO (SWD pad) | SWD data            |
| GPIO25         | SWCLK (SWD pad) | SWD clock           |
| USB            | USB port        | Power + ground only |

### UART Pins

RX and TX are standard header pins on the Feather M0 (pins 0 and 1). Connect with dupont jumper wires.

### SWD Pads

The SWD pads are small test points on the **bottom** of the Feather M0 board, labeled **SWDIO** and **SWCLK**. Options for connecting:

- **Solder wires directly** — most reliable for a permanent setup. Use thin wire (30 AWG silicone). Add hot glue for strain relief.
- **Pogo pin jig** — spring-loaded pogo pins (P75-B1, 1.0mm tip) in a 3D-printed jig. Good for tool-free connect/disconnect.
- **Solder header pins** — if the pad spacing allows, solder 0.1" pins and use dupont jumpers.

Keep SWD wires short (under 15 cm) to avoid signal issues.

## Serial

### RPi Setup

Enable the serial hardware and disable the login shell:

```bash
sudo raspi-config
# -> Interface Options -> Serial Port
# -> "Login shell over serial?" -> No
# -> "Serial port hardware enabled?" -> Yes
# Reboot
```

Then monitor with:

```bash
tio /dev/ttyS0 -b 115200
```

### Feather M0 Firmware

Use `Serial1` (hardware UART on pins 0/1), not `Serial` (USB CDC):

```cpp
Serial1.begin(115200);
Serial1.println("hello from feather");
```

The UART link stays up regardless of sleep state. When the Feather sleeps, data stops. When it wakes and writes to `Serial1`, output appears immediately.

## Flashing

Uses SWD via OpenOCD. Works regardless of sleep mode or firmware state.

```bash
sudo apt install openocd
```

Flash a `.bin` file (offset `0x00002000` preserves the UF2 bootloader):

```bash
openocd -f interface/raspberrypi-native.cfg \
        -f target/at91samdXX.cfg \
        -c "program firmware.bin verify reset exit 0x00002000"
```

Flash an `.elf` file (addresses are embedded in the ELF, no offset needed):

```bash
openocd -f interface/raspberrypi-native.cfg \
        -f target/at91samdXX.cfg \
        -c "program firmware.elf verify reset exit"
```

OpenOCD resets the chip after flashing via SWD — no physical reset button or GPIO reset wire needed.
