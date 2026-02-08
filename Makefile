GO ?= go

# Pure-Go packages testable with standard go toolchain.
TEST_PKGS = \
	./internal/config/ \
	./internal/log/ \
	./internal/manager/ \
	./internal/sensor/... \
	./internal/sink/...

.PHONY: test vet clean

test:
	$(GO) test $(TEST_PKGS)

vet:
	$(GO) vet $(TEST_PKGS)

clean:
	$(MAKE) -C targets/hypnos-m0 clean
