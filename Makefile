TARGET = feather-m0
BIN = main.bin

# User must provide BOSSAC (path to bossac), e.g.
# make flash BOSSAC=/path/to/bossac
BOSSAC ?=

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
	if [ -z "$(BOSSAC)" ]; then \
		echo "BOSSAC is not set. Example:"; \
		echo "  make flash BOSSAC=/absolute/path/to/bossac"; \
		exit 1; \
	fi; \
	if [ -z "$(PORT)" ]; then \
		echo "No board found. Plug it in (or double-tap reset) and try again."; \
		exit 1; \
	fi; \
	echo "Flashing on $(PORT)..."; \
	"$(BOSSAC)" -p "$(PORT)" -e -w -v "$(BIN)" -R

clean:
	rm -f $(BIN)

monitor:
	screen $(PORT) 115200
