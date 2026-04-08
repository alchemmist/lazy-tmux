#!/bin/sh

apt update && apt install -y wget tar make git gcc build-essential \
    libevent-dev \
    libncurses-dev \
    bison \
    pkg-config \
    wget \
    zsh \
    git \
    vim \
    curl \
    tree \
    locales \
    ncurses-term \
    htop \

TMUX_VERSION=${TMUX_VERSION:-3.6a}

cd /tmp && \
    wget https://github.com/tmux/tmux/releases/download/${TMUX_VERSION}/tmux-${TMUX_VERSION}.tar.gz && \
    tar -xzf tmux-${TMUX_VERSION}.tar.gz && \
    cd tmux-${TMUX_VERSION} && \
    ./configure && \
    make && \
    make install && \
    cd / && \
    rm -rf /tmp/tmux-${TMUX_VERSION}*
