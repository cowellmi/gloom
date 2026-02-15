FROM docker.io/library/golang:1.24-bullseye

RUN apt-get update && apt-get install -y \
    curl \
    wget \
    ca-certificates \
    git \
    && wget -q https://github.com/tinygo-org/tinygo/releases/download/v0.40.1/tinygo_0.40.1_amd64.deb \
    && dpkg -i tinygo_0.40.1_amd64.deb \
    && rm tinygo_0.40.1_amd64.deb \
    && apt-get clean

RUN useradd -m -s /bin/bash gloom
USER gloom
WORKDIR /workspace

RUN curl -fsSL https://cursor.com/install | bash

ENV PATH="/home/gloom/.local/bin:/usr/local/tinygo/bin:/usr/local/go/bin:${PATH}"

ENTRYPOINT ["agent"]
CMD ["--model", "auto"]
