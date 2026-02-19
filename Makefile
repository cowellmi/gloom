BIN 		?= gloom.bin
LOG	 		?= gloom.log
BOARD		?= $(GLOOM_BOARD)
PORT		?= $(GLOOM_PORT)
SERIAL_PORT ?= $(or $(GLOOM_SERIAL_PORT),$(PORT))

TEST_PKGS := ./internal/config/ ./internal/log/ ./internal/manager/ ./internal/sensor/... ./internal/sink/...

SOURCES := $(shell find cmd internal -name '*.go')

$(BIN): $(SOURCES)
	tinygo build -size=short -tags=no_lfn -target=$(BOARD) -o=$(BIN) ./cmd/gloom

.PHONY: flash clean test vet monitor

flash: $(BIN)
ifeq ($(PORT),)
	$(error PORT is not set: set GLOOM_PORT in your .envrc)
endif
	bossac --port="$(PORT)" --offset 0x2000 --erase --write --verify "$(BIN)"

clean:
	rm -f $(BIN)

test:
	go test $(TEST_PKGS)

vet:
	go vet $(TEST_PKGS)

monitor:
ifeq ($(SERIAL_PORT),)
	$(error SERIAL_PORT is not set: set GLOOM_SERIAL_PORT in your .envrc)
endif
	tio --log --log-file=$(LOG) $(SERIAL_PORT)
