-include .env
export GOTOOLCHAIN

MAIN		?= ./cmd/gloom
BIN 		?= ./build/gloom.bin
I2CTEST		?= ./cmd/i2ctest
I2CBIN		?= ./build/i2ctest.bin
LOG	 		?= ./debug.log
TARGET		?= $(GLOOM_TARGET)
PORT		?= $(GLOOM_PORT)
SERIAL_PORT ?= $(or $(GLOOM_SERIAL_PORT),$(PORT))
TAGS		?= $(GLOOM_TAGS)

SOURCES := $(shell find cmd internal -name '*.go')

$(BIN): $(SOURCES)
	tinygo build -size=short -stack-size=4KB -target=$(TARGET) -tags=$(TAGS) -o=$(BIN) $(MAIN)

$(I2CBIN): $(SOURCES)
	tinygo build -size=short -stack-size=4KB -target=$(TARGET) -tags=$(TAGS) -o=$(I2CBIN) $(I2CTEST)

.PHONY: flash flash-i2ctest clean test vet monitor

flash: $(BIN)
ifeq ($(PORT),)
	$(error PORT is not set: set GLOOM_PORT in your .env)
endif
	bossac --port="$(PORT)" --offset 0x2000 --erase --write --verify "$(BIN)"

flash-i2ctest: $(I2CBIN)
ifeq ($(PORT),)
	$(error PORT is not set: set GLOOM_PORT in your .env)
endif
	bossac --port="$(PORT)" --offset 0x2000 --erase --write --verify "$(I2CBIN)"

clean:
	rm -f $(BIN) $(I2CBIN)

test:
	go test ./...

vet:
	go vet ./...

monitor:
ifeq ($(SERIAL_PORT),)
	$(error SERIAL_PORT is not set: set GLOOM_SERIAL_PORT in your .env)
endif
	tio --log --log-file=$(LOG) $(SERIAL_PORT)
