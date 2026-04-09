#!/bin/sh

for version in "$@"; do
    url="https://github.com/tmux/tmux/releases/download/${version}/tmux-${version}.tar.gz"

    old_tmux=$(which tmux 2>/dev/null) || true
    if [ -n "$old_tmux" ]; then
        rm -f "$old_tmux"
    fi
    rm -f /usr/local/bin/tmux* /usr/local/share/man/man1/tmux.1
    rm -rf /tmp/tmux-* /tmp/tmux-*.tar.gz
    pkill -9 tmux 2>/dev/null || true
    sleep 1

    cd /tmp || { printf "  tmux %-7s  ✗ (cd /tmp failed)\n" "$version"; continue; }
    if ! curl -fsSL "$url" -o tmux.tar.gz 2>/dev/null; then
        printf "  tmux %-7s  ✗\n" "$version"
        cd / || true
        continue
    fi

    if ! tar -xzf tmux.tar.gz 2>/dev/null; then
        printf "  tmux %-7s  ✗\n" "$version"
        cd / || true
        continue
    fi

    cd "tmux-${version}" || { printf "  tmux %-7s  ✗ (cd tmux-%s failed)\n" "$version" "$version"; continue; }
    if ! ./configure >/dev/null 2>&1; then
        printf "  tmux %-7s  ✗\n" "$version"
        cd / || true
        continue
    fi

    if ! make -j"$(nproc)" >/dev/null 2>&1; then
        printf "  tmux %-7s  ✗\n" "$version"
        cd / || true
        continue
    fi

    if ! make install >/dev/null 2>&1; then
        printf "  tmux %-7s  ✗\n" "$version"
        cd / || true
        continue
    fi

    cd / || true

    actual_version=$(tmux -V 2>&1 | grep -o '[0-9][0-9.]*[a-z]*' | head -1)
    if [ "$actual_version" != "$version" ]; then
        printf "  tmux %-7s  ✗\n" "$version"
        continue
    fi

    if ! tmux new-session -d -s vtest 'sleep 30' 2>/dev/null; then
        printf "  tmux %-7s  ✗\n" "$version"
        continue
    fi
    sleep 1

    if ! lazy-tmux save --all --scrollback >/dev/null 2>&1; then
        tmux kill-server 2>/dev/null || true
        rm -rf ~/.local/share/lazy-tmux
        printf "  tmux %-7s  ✗\n" "$version"
        continue
    fi

    if lazy-tmux list 2>/dev/null | grep -q vtest; then
        printf "  tmux %-7s  ✓\n" "$version"
    else
        printf "  tmux %-7s  ✗\n" "$version"
    fi

    tmux kill-server 2>/dev/null || true
    rm -rf ~/.local/share/lazy-tmux
done
