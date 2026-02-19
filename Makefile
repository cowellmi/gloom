MAIN		?= ./cmd/gloom
BIN 		?= ./build/gloom.bin
I2CTEST		?= ./cmd/i2ctest
I2CBIN		?= ./build/i2ctest.bin
LOG	 		?= ./debug.log
BOARD		?= $(GLOOM_BOARD)
PORT		?= $(GLOOM_PORT)
SERIAL_PORT ?= $(or $(GLOOM_SERIAL_PORT),$(PORT))
TAGS		?= $(GLOOM_TAGS)

TEST_PKGS := ./internal/config/ ./internal/log/ ./internal/manager/ ./internal/sensor/... ./internal/sink/...

SOURCES := $(shell find cmd internal -name '*.go')

$(BIN): $(SOURCES)
	tinygo build -size=short -stack-size=8KB -target=$(BOARD) -tags=$(TAGS) -o=$(BIN) $(MAIN)

$(I2CBIN): $(SOURCES)
	tinygo build -size=short -stack-size=8KB -target=$(BOARD) -tags=$(TAGS) -o=$(I2CBIN) $(I2CTEST)

.PHONY: flash flash-i2ctest clean test vet monitor

flash: $(BIN)
ifeq ($(PORT),)
	$(error PORT is not set: set GLOOM_PORT in your .envrc)
endif
	bossac --port="$(PORT)" --offset 0x2000 --erase --write --verify "$(BIN)"

flash-i2ctest: $(I2CBIN)
ifeq ($(PORT),)
	$(error PORT is not set: set GLOOM_PORT in your .envrc)
endif
	bossac --port="$(PORT)" --offset 0x2000 --erase --write --verify "$(I2CBIN)"

clean:
	rm -f $(BIN) $(I2CBIN)

test:
	go test $(TEST_PKGS)

vet:
	go vet $(TEST_PKGS)

monitor:
ifeq ($(SERIAL_PORT),)
	$(error SERIAL_PORT is not set: set GLOOM_SERIAL_PORT in your .envrc)
endif
	tio --log --log-file=$(LOG) $(SERIAL_PORT)
