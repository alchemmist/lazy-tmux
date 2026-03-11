<h2><img src="./assets/logo.svg" alt="Favicon Preview" width="50" align="center">&nbsp;&nbsp;&nbsp;lazy-tmux</h2>

`lazy-tmux` is a Go CLI that snapshots tmux sessions to disk and restores them lazily on demand.

## Why

- Keeps session state across reboots.
- Restores only the session you choose (no eager full restore).
- Supports periodic autosave and manual save.

## Features

- `save` current session, specific session, or all sessions.
- `daemon` mode for periodic autosave (single-instance lock per tmux socket).
- `picker` with built-in TUI tree (`session -> windows`) and fuzzy search in a tmux popup (default).
- Optional `fzf` picker backend via `--fzf-engine`.
- Optional shell pane scrollback capture/replay (`--scrollback`, `--scrollback-lines`).
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

```

# Installation

Install with builtin powerful TUI picker:

```bash
curl -fsSL https://alchemmist.github.io/lazy-tmux/install.sh | sh
```

Install pure, no-deps, lightweight binary (fzf required):

```bash
curl -fsSL https://alchemmist.github.io/lazy-tmux/install.sh | sh -s -- --fzf-engine
```

Homebrew:

```bash
brew install alchemmist/tap/lazy-tmux
```

AUR (yay):

```bash
yay -S lazy-tmux
```

# Lazy restore picker in popup
```tmux
bind-key f display-popup -E -w 100% -h 100% 'lazy-tmux picker'
```

After reloading tmux config (`tmux source-file ~/.tmux.conf`):

- `prefix + s` saves snapshots.
- `prefix + f` opens TUI picker from saved sessions/windows (`--fzf-engine` for `fzf` backend).
- selected session is restored only when selected.

## CLI

```bash
lazy-tmux save [--all] [--session NAME] [--data-dir DIR] [--scrollback] [--scrollback-lines N]
lazy-tmux restore --session NAME [--switch=true]
lazy-tmux picker [--fzf-engine] [--session-sort EXPR] [--window-sort EXPR]
lazy-tmux bootstrap [--session last|NAME]
lazy-tmux daemon [--interval 5m] [--scrollback] [--scrollback-lines N]
lazy-tmux list
```

## Picker sorting

`picker` supports configurable multi-key sorting with priority control.

Flags:

- `--session-sort EXPR` controls session order.
- `--window-sort EXPR` controls window order inside each session.

Expression format:

- `EXPR` is a comma-separated list of keys.
- each key is `field` or `field:asc` or `field:desc`.
- order of keys in `EXPR` is the sort priority (leftmost key is highest priority).

Examples:

```bash
# Sort sessions by name, then by captured time (newest first)
lazy-tmux picker --session-sort "name:asc,captured:desc"

# Sort windows by pane count, then by name
lazy-tmux picker --window-sort "panes:desc,name:asc"

# Use same sorting with fzf backend
lazy-tmux picker --fzf-engine --session-sort "last-used:desc,name:asc"
```

Session sort fields:

- `last-used` (alias: `last-accessed`, `last_accessed`)
- `captured` (alias: `captured-at`, `captured_at`)
- `name`
- `windows`
- `panes`

Window sort fields:

- `index`
- `name`
- `panes`
- `cmd`

Default directions (when `:asc|:desc` is omitted):

- sessions: `name=asc`, all other session fields = `desc`
- windows: `index=asc`, `name=asc`, all other window fields = `desc`

Current defaults (if no sort flags are passed):

- sessions: `last-used:desc,captured:desc,name:asc`
- windows: `index:asc,name:asc`

Validation behavior:

- unknown fields are rejected with an error.
- invalid direction values are rejected (`asc` and `desc` only).
- duplicate fields in one expression are rejected.

## Shell scrollback

By default, scrollback capture is disabled.

Enable it explicitly:

```bash
lazy-tmux save --all --scrollback --scrollback-lines 5000
lazy-tmux daemon --interval 5m --scrollback --scrollback-lines 5000
```

Behavior:

- captures tmux pane scrollback only for panes that currently run an interactive shell (no detected foreground app command).
- stores scrollback as sidecar files and references them from session snapshots.
- on restore, writes captured scrollback back into pane tty before command replay.

Storage layout:

- `~/.local/share/lazy-tmux/sessions/*.json`
- `~/.local/share/lazy-tmux/scrollback/<session>/*.log`

## Storage

Default directory:

- `~/.local/share/lazy-tmux/index.json`
- `~/.local/share/lazy-tmux/sessions/*.json`
- `~/.local/share/lazy-tmux/scrollback/*`

Override via:

- env: `LAZY_TMUX_DATA_DIR`
- flag: `--data-dir`

## Important behavior notes

- This tool restores tmux structure (sessions/windows/panes/layouts and pane commands when available).
- It does **not** checkpoint process memory state; long-running interactive processes are restarted only if tmux exposes enough pane command metadata for recreation.

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
