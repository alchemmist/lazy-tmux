FROM ghcr.io/charmbracelet/vhs:latest
RUN apt-get update && apt-get install -y --no-install-recommends tmux procps \
    && rm -rf /var/lib/apt/lists/*
COPY lazy-tmux /usr/local/bin/lazy-tmux
WORKDIR /root
ENV TERM=xterm-256color
