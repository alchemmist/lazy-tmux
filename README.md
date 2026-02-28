# lazy-tmux

`lazy-tmux` is a Go CLI that snapshots tmux sessions to disk and restores them lazily on demand.

## Why

- Keeps session state across reboots.
- Restores only the session you choose (no eager full restore).
- Supports periodic autosave and manual save.

## Features

- `save` current session, specific session, or all sessions.
- `daemon` mode for periodic autosave (single-instance lock per tmux socket).
- `picker` with `fzf` for fast session search in a tmux popup.
- `restore` exactly one selected session from disk.
- `bootstrap` on tmux startup to restore latest or named session.

## Install

### From source

```bash
make install
```

or

```bash
go install ./cmd/lazy-tmux
```

### Build local binary

```bash
make build
```

Binary path: `bin/lazy-tmux`.

## tmux setup

Add this to your `~/.tmux.conf`:

```tmux
# Start lazy autosave daemon (every 5 minutes)
run-shell -b 'lazy-tmux daemon --interval 5m >/tmp/lazy-tmux.log 2>&1'

# Restore one session on startup (latest snapshot)
run-shell -b 'lazy-tmux bootstrap --session last >/tmp/lazy-tmux-bootstrap.log 2>&1'

# Manual save
bind-key s run-shell 'lazy-tmux save --all'

# Lazy restore picker in popup
bind-key f display-popup -E 'lazy-tmux picker'
```

After reloading tmux config (`tmux source-file ~/.tmux.conf`):

- `prefix + s` saves snapshots.
- `prefix + f` opens `fzf` picker from saved sessions.
- selected session is restored only when selected.

## CLI

```bash
lazy-tmux save [--all] [--session NAME] [--data-dir DIR]
lazy-tmux restore --session NAME [--switch=true]
lazy-tmux picker
lazy-tmux bootstrap [--session last|NAME]
lazy-tmux daemon [--interval 5m]
lazy-tmux list
```

## Storage

Default directory:

- `~/.local/share/lazy-tmux/index.json`
- `~/.local/share/lazy-tmux/sessions/*.json`

Override via:

- env: `LAZY_TMUX_DATA_DIR`
- flag: `--data-dir`

## Important behavior notes

- This tool restores tmux structure (sessions/windows/panes/layouts and pane start commands when available).
- It does **not** checkpoint process memory state; long-running interactive processes are restarted only if tmux exposes a start command for pane recreation.

## Development

```bash
make fmt
make lint
make test
make build
make check
```

## Release

`goreleaser` config is included.

```bash
goreleaser release --snapshot --clean
```
