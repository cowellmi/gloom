#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_HOME=/home/agent

IMAGE="docker.io/tinygo/tinygo:0.40.1"
VOLUME_NAME="cursor-agent-cache-$(basename "$PROJECT_DIR")"

CURSOR_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/cursor"

# Ensure config exists
mkdir -p "$CURSOR_CONFIG"

# Use distrobox-host-exec if inside distrobox
if [[ -v DISTROBOX_ENTER_PATH ]]; then
    PODMAN="distrobox-host-exec podman"
else
    PODMAN="podman"
fi

# Ensure volume exists
$PODMAN volume inspect "$VOLUME_NAME" &>/dev/null \
    || $PODMAN volume create "$VOLUME_NAME" >/dev/null

exec $PODMAN run -it --rm \
    --init \
    --userns keep-id \
    --env "HOME=$CONTAINER_HOME" \
    --env "GOPATH=$CONTAINER_HOME/go" \
    --env "GOCACHE=$CONTAINER_HOME/.cache/go-build" \
    --env "PATH=/usr/local/go/bin:/tinygo/bin:$CONTAINER_HOME/go/bin:$CONTAINER_HOME/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
    -v "$VOLUME_NAME:$CONTAINER_HOME" \
    -v "$PROJECT_DIR:/workspace:Z" \
    -v "$CURSOR_CONFIG:$CONTAINER_HOME/.config/cursor:ro,Z" \
    -w /workspace \
    "$IMAGE" \
    /bin/bash -c '
        set -e

        mkdir -p \
          "$HOME/.local/bin" \
          "$GOPATH" \
          "$GOCACHE"

        # Install Cursor agent if not already present.
        if [[ ! -x "$HOME/.local/bin/agent" ]]; then
            curl -fsSL https://cursor.com/install | bash
        fi

        exec agent --model auto "$@"
    ' _ "$@"
