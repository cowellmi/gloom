image_name := "gloom-agent"

# Build the gloom-agent image
agent-build:
    podman build -t {{image_name}} .

# Run the gloom-agent in the current directory
agent *args:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        -v "$HOME/.config/cursor:/home/gloom/.config/cursor:ro,Z" \
        {{image_name}} {{args}}

# Debug shell
agent-shell:
    podman run -it --rm \
        --init \
        --userns keep-id \
        -v "$(pwd):/workspace:Z" \
        --entrypoint /bin/bash \
        {{image_name}}
