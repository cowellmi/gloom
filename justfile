
test_pkgs := "./internal/config/ ./internal/log/ ./internal/manager/ ./internal/sensor/... ./internal/sink/..."

test:
    go test {{test_pkgs}}

vet:
    go vet {{test_pkgs}}

clean:
    just -f targets/feather-m0/justfile clean

monitor:
    tio --log --log-file="debug.log" /dev/tty.usbserial-AI04YQAD

# Expiremental: sandboxed coding agent.
image_name := "gloom-agent"

agent-build:
    podman build -t {{image_name}} .

agent *args:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        -v "$HOME/.config/cursor:/home/gloom/.config/cursor:ro,Z" \
        {{image_name}} {{args}}

agent-shell:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        --entrypoint /bin/bash \
        {{image_name}}
