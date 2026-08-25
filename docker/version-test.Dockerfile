# Image for the tmux version-support matrix (scripts/test-tmux-versions.sh).
#
# Unlike docker/sandbox.Dockerfile — the public demo image, which installs the
# published lazy-tmux via the website install script — this builds lazy-tmux
# from the repository source. That way the matrix validates the code under
# review (not a released binary) and does not depend on lazy-tmux.xyz being up.

# --- build stage: compile lazy-tmux from source ---
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static build so the binary runs on the ubuntu runtime stage as-is.
RUN CGO_ENABLED=0 go build -o /out/lazy-tmux ./cmd/lazy-tmux

# --- runtime stage: toolchain to build each tmux release at runtime ---
FROM ubuntu:26.04
ENV DEBIAN_FRONTEND=noninteractive

# build-essential … bison: compile tmux from source inside the container.
# vim, procps: the realistic fixture runs vim/top as long-lived pane programs.
# util-linux: script/setsid for the best-effort attached client.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libevent-dev \
    libncurses-dev \
    bison \
    pkg-config \
    wget \
    curl \
    git \
    ca-certificates \
    util-linux \
    vim \
    procps \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/lazy-tmux /usr/local/bin/lazy-tmux

COPY docker/test-versions-inner.sh /test-versions-inner.sh
RUN chmod +x /test-versions-inner.sh
