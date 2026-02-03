TARGET = feather-m0
BIN    = main.bin
OFFSET = 0x2000

# Auto-detect serial port (macOS + Linux)
PORT ?= $(shell ls \
	/dev/cu.usbmodem* \
	/dev/cu.usbserial* \
	/dev/ttyACM* \
	/dev/ttyUSB* \
	2>/dev/null | head -n 1)

.PHONY: flash clean

build:
	tinygo build -target=$(TARGET) -o $(BIN) .

flash: build
	@set -e; \
	if [ -z "$(PORT)" ]; then \
		echo "No board found. Plug it in (or double-tap reset) and try again."; \
		exit 1; \
	fi; \
	echo "Flashing on $(PORT)..."; \
	bossac -p $(PORT) -a -o $(OFFSET) -e -w -v -b $(BIN) -R

clean:
	rm -f $(BIN)
