<h2>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./docs/public/assets/logo-white.svg">
    <img src="./docs/public/assets/logo.svg" alt="Favicon Preview" width="60" align="center">
  </picture>
  &nbsp;&nbsp;&nbsp;lazy-tmux
</h2>

[![quality](https://github.com/alchemmist/lazy-tmux/actions/workflows/quality-graph.yml/badge.svg?branch=main)](https://github.com/alchemmist/lazy-tmux/actions/workflows/quality-graph.yml)
[![release](https://img.shields.io/github/v/release/alchemmist/lazy-tmux)](https://github.com/alchemmist/lazy-tmux/releases/latest)
[![homebrew](https://img.shields.io/badge/homebrew-alchemmist%2Ftap%2Flazy--tmux-FBB040?logo=homebrew&logoColor=white)](https://github.com/alchemmist/homebrew-tap)
[![AUR](https://img.shields.io/aur/version/lazy-tmux?logo=archlinux)](https://aur.archlinux.org/packages/lazy-tmux)
[![Docker pulls](https://img.shields.io/docker/pulls/alchemmist/lazy-tmux?logo=docker)](https://hub.docker.com/r/alchemmist/lazy-tmux)
[![Go](https://img.shields.io/github/go-mod/go-version/alchemmist/lazy-tmux?logo=go)](go.mod)
[![license](https://img.shields.io/github/license/alchemmist/lazy-tmux)](LICENSE)
[![platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux-8a8a8a)](https://lazy-tmux.xyz/installation/)

Project architect: [@alchemmist](https://github.com/alchemmist)

CLI written in Go for saving and restoring tmux sessions lazily. Key features:

- Save sessions: current, specific, or all — including windows, panes, layouts, running commands, and scrollback history.
- Lazy restore: restore only what you need, avoiding high RAM usage.
- Autosave daemon: periodically snapshots all sessions in the background (single instance, no conflicts).
- Interactive TUI browser: tree view (sessions/windows) + table (commands, snapshot time, counts, status) with fuzzy search.
- Keyboard-driven picker for fast search, navigation, and manage sessions and windows directly inside picker tree.
- Lightweight `picker --sessions-only` mode for Alt-Tab-style switching in a narrow tmux popup.
- In the picker, press `Option/Alt` + a window number (`1`, `2`, `3`, …) to immediately restore the first matching window from the current results.
- Codex and Claude Code integrations detect the active session on every save and restore the exact session instead of starting a new conversation.
- Flexible sorting via `--session-sort` or `--window-sort` (by last-used, time, size, name, command, etc.).
- Optional `fzf` integration via `--fzf-engine` (lighter and no dependencies binary, but without full keyboard control and TUI picker); add `--windows` to pick a specific window instead of a whole session.
- Bootstrap restore on tmux startup: auto-restore latest or specific session.
- Full environment snapshots: restore pane layout and commands (e.g. `npm`, `docker-compose`, `nvim`).
- Optional scrollback capture: preserve and replay previous terminal output.

Save your sessions, kill the entire tmux server, then bring everything back from the TUI picker:

![lazy-tmux demo — save, kill-server, and restore via the picker](./docs/public/assets/demo.gif)

Check out [lazy-tmux.xyz](https://lazy-tmux.xyz) for more information about installation and usage!

Just for building from source you need to have installed go and cloned this project. After that run:

```bash
make build
```

Binary will be compiled in `bin/lazy-tmux`. For more development options check out `Makefile` tasks.

> [!NOTE]
> **tmux versions:** lazy-tmux supports every tmux release from **2.9 through 3.7b**,
> verified on each one by the CI version matrix. Newer releases are added as they ship.

For configuration, CLI reference, and usage, see the docs at
[lazy-tmux.xyz](https://lazy-tmux.xyz).
