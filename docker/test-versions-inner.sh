#!/bin/sh
#
# Per-version tmux compatibility check, run inside the sandbox image by
# scripts/test-tmux-versions.sh. For each tmux version passed as an argument it
# builds and installs that tmux from source, then exercises lazy-tmux against a
# REALISTIC session and asserts the save/list round-trip is faithful.
#
# A version is reported supported (✓) only when:
#   * save and list run with a zero exit code AND no error output, and
#   * the saved snapshot reflects the live session (window and pane counts).
#
# This is deliberately stricter than the old check, which started a single
# `sleep 30` pane, swallowed all save/list output, and marked ✓ on a bare
# `grep vtest`. That hid real failures (see issues #125 / #126): a version could
# fail on a real multi-window, multi-pane, attached session yet still pass.
#
# Output contract (consumed by scripts/quality-graph-tmux-versions.sh):
# exactly one "  tmux <version>  ✓" or "  tmux <version>  ✗" line per version on
# stdout. Failure diagnostics go to stderr so they appear in the raw CI log
# without polluting the parsed summary.

set -u

# The fixture below builds a session with three windows; window 0 holds two
# panes, so four panes total.
EXPECT_WINDOWS=3
EXPECT_PANES=4

log() { printf '      %s\n' "$1" >&2; }

# Build and install one tmux version from source, replacing any currently
# installed tmux. Returns non-zero (with a diagnostic) on any failure.
build_tmux() {
    version=$1
    url="https://github.com/tmux/tmux/releases/download/${version}/tmux-${version}.tar.gz"

    old_tmux=$(command -v tmux 2>/dev/null) || true
    [ -n "$old_tmux" ] && rm -f "$old_tmux"
    rm -f /usr/local/bin/tmux* /usr/local/share/man/man1/tmux.1
    rm -rf /tmp/tmux-* /tmp/tmux-*.tar.gz
    pkill -9 tmux 2>/dev/null || true
    sleep 1

    cd /tmp || { log "cd /tmp failed"; return 1; }
    curl -fsSL --connect-timeout 15 --max-time 300 "$url" -o tmux.tar.gz 2>/dev/null || { log "download failed: $url"; return 1; }
    tar -xzf tmux.tar.gz 2>/dev/null || { log "extract failed"; return 1; }
    cd "tmux-${version}" || { log "cd tmux-${version} failed"; return 1; }
    ./configure >/dev/null 2>&1 || { log "configure failed"; return 1; }
    make -j"$(nproc)" >/dev/null 2>&1 || { log "make failed"; return 1; }
    make install >/dev/null 2>&1 || { log "make install failed"; return 1; }
    cd / || true

    actual=$(tmux -V 2>&1 | grep -o '[0-9][0-9.]*[a-z]*' | head -1)
    [ "$actual" = "$version" ] || { log "version mismatch: wanted $version, got '$actual'"; return 1; }
    return 0
}

# Build a session that resembles a real user's, not a single throwaway pane:
# multiple windows, multiple panes, real shell panes running long-lived
# foreground programs, and a best-effort attached client.
build_fixture() {
    tmux kill-server 2>/dev/null || true
    rm -rf "$HOME/.local/share/lazy-tmux"

    # window 0 "editor": two shell panes, each running a long-lived program.
    tmux new-session -d -s vtest -n editor -x 200 -y 50 || { log "new-session failed"; return 1; }
    tmux send-keys -t vtest:editor 'vim' C-m
    tmux split-window -h -t vtest:editor || { log "split-window failed"; return 1; }
    tmux send-keys -t vtest:editor.1 'tail -f /dev/null' C-m

    # window 1 "top": a long-lived foreground program.
    tmux new-window -t vtest -n top || { log "new-window top failed"; return 1; }
    tmux send-keys -t vtest:top 'top' C-m

    # window 2 "shell": a plain interactive shell.
    tmux new-window -t vtest -n shell || { log "new-window shell failed"; return 1; }

    # Best-effort: attach a client through a pseudo-tty so the session reports
    # as (attached), mirroring a real session. Never fatal if unavailable.
    if command -v script >/dev/null 2>&1 && command -v setsid >/dev/null 2>&1; then
        setsid sh -c 'script -q -c "tmux attach -t vtest" /dev/null' >/dev/null 2>&1 &
    fi

    sleep 2  # let the foreground programs come up
    return 0
}

# Save the fixture and assert the snapshot is faithful. Diagnostics to stderr.
check_roundtrip() {
    live_windows=$(tmux list-windows -t vtest 2>/dev/null | wc -l | tr -d ' ')
    live_panes=$(tmux list-panes -s -t vtest 2>/dev/null | wc -l | tr -d ' ')

    # Run save WITHOUT swallowing output: any stderr or non-zero exit fails.
    save_err=$(lazy-tmux save --all --scrollback 2>&1 >/dev/null)
    save_rc=$?
    if [ "$save_rc" -ne 0 ] || [ -n "$save_err" ]; then
        log "save failed (rc=$save_rc): ${save_err:-<no output>}"
        return 1
    fi

    list_out=$(lazy-tmux list 2>&1)
    list_rc=$?
    if [ "$list_rc" -ne 0 ]; then
        log "list failed (rc=$list_rc): ${list_out:-<no output>}"
        return 1
    fi

    vtest_line=$(printf '%s\n' "$list_out" | grep '^vtest')
    if [ -z "$vtest_line" ]; then
        log "vtest missing from list output: ${list_out:-<empty>}"
        return 1
    fi

    # list format: "<name>\t<timestamp>\t<W>w/<P>p" (see cmd/lazy-tmux/list.go).
    counts=$(printf '%s' "$vtest_line" | awk -F'\t' '{print $3}')
    saved_windows=$(printf '%s' "$counts" | sed 's/w.*//')
    saved_panes=$(printf '%s' "$counts" | sed 's|.*/||; s/p$//')

    case "$saved_windows" in
        ''|*[!0-9]*) log "unparseable window count in: $vtest_line"; return 1 ;;
    esac
    case "$saved_panes" in
        ''|*[!0-9]*) log "unparseable pane count in: $vtest_line"; return 1 ;;
    esac

    if [ "$saved_windows" -lt "$live_windows" ]; then
        log "saved windows ($saved_windows) < live ($live_windows); line: $vtest_line"
        return 1
    fi
    if [ "$saved_panes" -lt "$live_panes" ]; then
        log "saved panes ($saved_panes) < live ($live_panes); line: $vtest_line"
        return 1
    fi
    if [ "$saved_windows" -lt "$EXPECT_WINDOWS" ]; then
        log "saved windows ($saved_windows) < expected $EXPECT_WINDOWS; line: $vtest_line"
        return 1
    fi
    # Absolute floor mirroring EXPECT_WINDOWS: if the live pane query failed,
    # live_panes is 0 and the >= live check is vacuous, so assert against the
    # known fixture size too.
    if [ "$saved_panes" -lt "$EXPECT_PANES" ]; then
        log "saved panes ($saved_panes) < expected $EXPECT_PANES; line: $vtest_line"
        return 1
    fi

    return 0
}

check_version() {
    version=$1

    build_tmux "$version" || return 1
    build_fixture || return 1
    check_roundtrip
}

failed=0
for version in "$@"; do
    if check_version "$version"; then
        printf "  tmux %-7s  ✓\n" "$version"
    else
        printf "  tmux %-7s  ✗\n" "$version"
        failed=1
    fi
    tmux kill-server 2>/dev/null || true
    rm -rf "$HOME/.local/share/lazy-tmux"
done

exit "$failed"
