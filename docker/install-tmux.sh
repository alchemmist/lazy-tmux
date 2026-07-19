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
TARBALL="tmux-${TMUX_VERSION}.tar.gz"
URL="https://github.com/tmux/tmux/releases/download/${TMUX_VERSION}/${TARBALL}"

cd /tmp || exit 1

ok=0
i=1
while [ "$i" -le 5 ]; do
    if wget --tries=3 --timeout=30 -O "$TARBALL" "$URL" \
        || curl -fSL --retry 3 --connect-timeout 30 -o "$TARBALL" "$URL"; then
        ok=1
        break
    fi
    echo "tmux download attempt $i failed; retrying..." >&2
    i=$((i + 1))
    sleep 3
done

if [ "$ok" -ne 1 ]; then
    echo "failed to download $URL after 5 attempts" >&2
    exit 1
fi

tar -xzf "$TARBALL" && \
    cd "tmux-${TMUX_VERSION}" && \
    ./configure && \
    make && \
    make install && \
    cd / && \
    rm -rf "/tmp/tmux-${TMUX_VERSION}"*
