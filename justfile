default:
    @just --list

test_pkgs := "./internal/config/ ./internal/log/ ./internal/manager/ ./internal/sensor/... ./internal/sink/..."

test:
    go test {{test_pkgs}}

vet:
    go vet {{test_pkgs}}

board := "feather-m0"
bin := "gloom.bin"
log := "gloom.log"

build:
	tinygo build -size=short -stack-size=8KB -target={{board}} -o={{bin}} ./cmd/gloom

# Example: just flash /dev/ttyACM0 --reset
flash port=env_var("GLOOMPORT"): build
    bossac --port="{{port}}" --offset=0x2000 --erase --write --verify "{{bin}}"

clean:
	rm -f {{bin}}

monitor port=env_var("GLOOMPORT"):
    tio --log --log-file="gloom.log" {{port}}

# Expiremental: sandboxed coding agent.
image_name := "gloom-agent"

agent-build:
    podman build -t {{image_name}} .

agent:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        -v "$HOME/.config/cursor:/home/gloom/.config/cursor:ro,Z" \
        {{image_name}}

agent-shell:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        --entrypoint /bin/bash \
        {{image_name}}
