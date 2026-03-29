FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && \
    apt install -y curl vim tree htop tmux zsh && \
    rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://lazy-tmux.xyz/install.sh | sh

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV SHELL=/usr/bin/zsh

ENTRYPOINT ["/entrypoint.sh"]
