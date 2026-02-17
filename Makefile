BIN 	?= gloom.bin
LOG	 	?= gloom.log
BOARD	?= $(GLOOMBOARD)
PORT	?= $(GLOOMPORT)
OFFSET	?= $(GLOOMOFFSET)

TEST_PKGS := ./internal/config/ ./internal/log/ ./internal/manager/ ./internal/sensor/... ./internal/sink/...

SOURCES := $(shell find cmd internal -name '*.go')

$(BIN): $(SOURCES)
	tinygo build -size=short -stack-size=8KB -target=$(BOARD) -o=$(BIN) ./cmd/gloom

.PHONY: flash clean test vet monitor

flash: $(BIN)
	bossac --port="$(PORT)" --offset=0x2000 --erase --write --verify "$(BIN)"

clean:
	rm -f $(BIN)

test:
	go test $(TEST_PKGS)

vet:
	go vet $(TEST_PKGS)

monitor:
	tio --log --log-file=$(LOG) $(PORT)
