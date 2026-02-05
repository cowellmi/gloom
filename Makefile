TARGET = feather-m0
BIN = main.bin
BOSSAC ?= $(shell which bossac)
PORT ?= $(shell ls \
	/dev/cu.usbmodem* \
	/dev/cu.usbserial* \
	/dev/tty.usbmodem* \
	/dev/tty.usbserial* \
	/dev/ttyACM* \
	/dev/ttyUSB* \
	2>/dev/null | head -n 1)

.PHONY: build flash clean monitor

build:
	tinygo build -size=short -target=$(TARGET) -o=$(BIN) .

flash: build
	@set -e; \
	if [ -z "$(PORT)" ]; then \
		echo "No board found. Plug it in (or double-tap reset) and try again."; \
		exit 1; \
	fi; \
	echo "Flashing on $(PORT)..."; \
	$(BOSSAC) -p "$(PORT)" -e -w -v "$(BIN)" -R

clean:
	rm -f $(BIN)

monitor:
	screen $(PORT) 115200
