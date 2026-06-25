<h2>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./docs/assets/logo-white.svg">
    <img src="./docs/assets/logo.svg" alt="Favicon Preview" width="60" align="center">
  </picture>
  &nbsp;&nbsp;&nbsp;lazy-tmux
</h2>

![Static Badge](https://img.shields.io/badge/website-red?style=flat&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPD94bWwgdmVyc2lvbj0iMS4wIiA%2FPgoNPCEtLSBVcGxvYWRlZCB0bzogU1ZHIFJlcG8sIHd3dy5zdmdyZXBvLmNvbSwgR2VuZXJhdG9yOiBTVkcgUmVwbyBNaXhlciBUb29scyAtLT4KPHN2ZyBmaWxsPSIjMDAwMDAwIiB3aWR0aD0iODAwcHgiIGhlaWdodD0iODAwcHgiIHZpZXdCb3g9IjAgMCA0MDAgNDAwIiBpZD0iTmlnaHQiIHZlcnNpb249IjEuMSIgeG1sOnNwYWNlPSJwcmVzZXJ2ZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIiB4bWxuczp4bGluaz0iaHR0cDovL3d3dy53My5vcmcvMTk5OS94bGluayI%2BCg08ZyBpZD0iWE1MSURfNDJfIj4KDTxwb2x5Z29uIGlkPSJYTUxJRF80NF8iIHBvaW50cz0iMTMzLjMsNTMuMyAxMzMuMywyNi43IDEwNi43LDI2LjcgODAsMjYuNyA4MCw1My4zIDEwNi43LDUzLjMgICIvPgoNPHBvbHlnb24gaWQ9IlhNTElEXzY0XyIgcG9pbnRzPSIxNjAsNTMuMyAxODYuNyw1My4zIDE4Ni43LDI2LjcgMjEzLjMsMjYuNyAyMTMuMywwIDE4Ni43LDAgMTYwLDAgMTMzLjMsMCAxMzMuMywyNi43IDE2MCwyNi43ICAgICAiLz4KDTxyZWN0IGhlaWdodD0iMjYuNyIgaWQ9IlhNTElEXzY1XyIgd2lkdGg9IjI2LjciIHg9IjUzLjMiIHk9IjUzLjMiLz4KDTxyZWN0IGhlaWdodD0iMjYuNyIgaWQ9IlhNTElEXzY2XyIgd2lkdGg9IjI2LjciIHg9IjEzMy4zIiB5PSI1My4zIi8%2BCg08cG9seWdvbiBpZD0iWE1MSURfOTBfIiBwb2ludHM9IjEwNi43LDEwNi43IDEwNi43LDEzMy4zIDEwNi43LDE2MCAxMDYuNywxODYuNyAxMDYuNywyMTMuMyAxMzMuMywyMTMuMyAxMzMuMywxODYuNyAxMzMuMywxNjAgICAgMTMzLjMsMTMzLjMgMTMzLjMsMTA2LjcgMTMzLjMsODAgMTA2LjcsODAgICIvPgoNPHBvbHlnb24gaWQ9IlhNTElEXzkxXyIgcG9pbnRzPSI1My4zLDEwNi43IDUzLjMsODAgMjYuNyw4MCAyNi43LDEwNi43IDI2LjcsMTMzLjMgNTMuMywxMzMuMyAgIi8%2BCg08cG9seWdvbiBpZD0iWE1MSURfOTJfIiBwb2ludHM9IjM3My4zLDE4Ni43IDM3My4zLDIxMy4zIDM0Ni43LDIxMy4zIDM0Ni43LDI0MCAzNzMuMywyNDAgMzczLjMsMjY2LjcgNDAwLDI2Ni43IDQwMCwyNDAgICAgNDAwLDIxMy4zIDQwMCwxODYuNyAgIi8%2BCg08cG9seWdvbiBpZD0iWE1MSURfOTNfIiBwb2ludHM9IjI2LjcsMjEzLjMgMjYuNywxODYuNyAyNi43LDE2MCAyNi43LDEzMy4zIDAsMTMzLjMgMCwxNjAgMCwxODYuNyAwLDIxMy4zIDAsMjQwIDAsMjY2LjcgICAgMjYuNywyNjYuNyAyNi43LDI0MCAgIi8%2BCg08cmVjdCBoZWlnaHQ9IjI2LjciIGlkPSJYTUxJRF85NF8iIHdpZHRoPSIyNi43IiB4PSIxMzMuMyIgeT0iMjEzLjMiLz4KDTxyZWN0IGhlaWdodD0iMjYuNyIgaWQ9IlhNTElEXzk1XyIgd2lkdGg9IjI2LjciIHg9IjE2MCIgeT0iMjQwIi8%2BCg08cmVjdCBoZWlnaHQ9IjI2LjciIGlkPSJYTUxJRF85Nl8iIHdpZHRoPSIyNi43IiB4PSIzMjAiIHk9IjI0MCIvPgoNPHBvbHlnb24gaWQ9IlhNTElEXzk3XyIgcG9pbnRzPSI1My4zLDI2Ni43IDI2LjcsMjY2LjcgMjYuNywyOTMuMyAyNi43LDMyMCA1My4zLDMyMCA1My4zLDI5My4zICAiLz4KDTxwb2x5Z29uIGlkPSJYTUxJRF85OF8iIHBvaW50cz0iMjEzLjMsMjkzLjMgMjQwLDI5My4zIDI2Ni43LDI5My4zIDI5My4zLDI5My4zIDMyMCwyOTMuMyAzMjAsMjY2LjcgMjkzLjMsMjY2LjcgMjY2LjcsMjY2LjcgICAgMjQwLDI2Ni43IDIxMy4zLDI2Ni43IDE4Ni43LDI2Ni43IDE4Ni43LDI5My4zICAiLz4KDTxwb2x5Z29uIGlkPSJYTUxJRF85OV8iIHBvaW50cz0iMzQ2LjcsMjkzLjMgMzQ2LjcsMzIwIDM3My4zLDMyMCAzNzMuMywyOTMuMyAzNzMuMywyNjYuNyAzNDYuNywyNjYuNyAgIi8%2BCg08cmVjdCBoZWlnaHQ9IjI2LjciIGlkPSJYTUxJRF8xMDBfIiB3aWR0aD0iMjYuNyIgeD0iNTMuMyIgeT0iMzIwIi8%2BCg08cmVjdCBoZWlnaHQ9IjI2LjciIGlkPSJYTUxJRF8xMDFfIiB3aWR0aD0iMjYuNyIgeD0iMzIwIiB5PSIzMjAiLz4KDTxwb2x5Z29uIGlkPSJYTUxJRF8xMDJfIiBwb2ludHM9IjEwNi43LDM0Ni43IDgwLDM0Ni43IDgwLDM3My4zIDEwNi43LDM3My4zIDEzMy4zLDM3My4zIDEzMy4zLDM0Ni43ICAiLz4KDTxwb2x5Z29uIGlkPSJYTUxJRF8xMDNfIiBwb2ludHM9IjI2Ni43LDM0Ni43IDI2Ni43LDM3My4zIDI5My4zLDM3My4zIDMyMCwzNzMuMyAzMjAsMzQ2LjcgMjkzLjMsMzQ2LjcgICIvPgoNPHBvbHlnb24gaWQ9IlhNTElEXzEwNF8iIHBvaW50cz0iMjEzLjMsMzczLjMgMTg2LjcsMzczLjMgMTYwLDM3My4zIDEzMy4zLDM3My4zIDEzMy4zLDQwMCAxNjAsNDAwIDE4Ni43LDQwMCAyMTMuMyw0MDAgMjQwLDQwMCAgICAyNjYuNyw0MDAgMjY2LjcsMzczLjMgMjQwLDM3My4zICAiLz4KDTwvZz4KDTwvc3ZnPg%3D%3D&color=%23add8e6&link=https%3A%2F%2Flazy-tmux.xyz)
![License](https://img.shields.io/github/license/alchemmist/devsyringe?style=flat)
![Contributors](https://img.shields.io/github/contributors/alchemmist/devsyringe?style=flat)
![Go](https://img.shields.io/badge/1.25-default?label=Go)
[![Build](https://github.com/alchemmist/lazy-tmux/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/alchemmist/lazy-tmux/actions/workflows/build.yml)

Project architect: [@alchemmist](https://github.com/alchemmist)

Cli written on Go for saving and restoring tmux sessions lazy. Key features:

- Save sessions: current, specific, or all — including windows, panes, layouts, running commands, and scrollback history.
- Lazy restore: restore only what you need, avoiding high RAM usage (unlike tmux-resurrect).
- Autosave daemon: periodically snapshots all sessions in the background (single instance, no conflicts).
- Interactive TUI browser: tree view (sessions/windows) + table (commands, snapshot time, counts, status) with fuzzy search.
- Keyboard-driven picker for fast search, navigation, and manage sessions and windows directly inside picker tree.
- Flexible sorting via `--session-sort` or `--window-sort` (by last-used, time, size, name, command, etc.).
- Optional `fzf` integration via `--fzf-engine` (lighter and no dependencies binary, but without full keyboard control and TUI picker); add `--windows` to pick a specific window instead of a whole session.
- Bootstrap restore on tmux startup: auto-restore latest or specific session.
- Full environment snapshots: restore pane layout and commands (e.g. `npm`, `docker-compose`, `nvim`).
- Optional scrollback capture: preserve and replay previous terminal output.

Chekout [lazy-tmux.xyz](https://lazy-tmux.xyz) for more informaiton about installation and usage!

Just for bulding from source you need to have installed go and cloned this project. After that run:

```bash
make build
```

Binary will compiled in `bin/lazy-tmux`. For more development options run `make help`.

> [!WARNING]
> **tmux version requirement:** lazy-tmux requires **tmux 3.6** or **tmux 3.6a**.
> Older versions may not work correctly.

## Configuration

lazy-tmux reads an optional TOML config file. It is looked up in this order:

1. `$LAZY_TMUX_CONFIG` (explicit path)
2. `$XDG_CONFIG_HOME/lazy-tmux/lazy-tmux.toml`
3. `~/.config/lazy-tmux/lazy-tmux.toml`

The file is optional — a missing file just uses the built-in defaults. Settings
are layered: **built-in defaults → config file → command-line flags**, so a flag
always overrides the file for that run.

```toml
# ~/.config/lazy-tmux/lazy-tmux.toml — all keys are optional

tmux_bin        = "tmux"      # tmux binary to use
data_dir        = "~/.local/share/lazy-tmux"  # where snapshots are stored
save_interval   = "5m"        # daemon autosave interval (Go duration)
restore_timeout = "5s"        # max wait for restored pane commands to start (0 disables)

# Allowlist of commands lazy-tmux may replay on restore, matched by program
# name. Omit the key to restore every command (default). Set it to restore only
# these programs; set it to [] to restore no commands at all.
restore_allowlist = ["nvim", "vim", "htop", "less", "tail", "ssh"]

[scrollback]
enabled = false               # capture shell pane scrollback
lines   = 5000                # max scrollback lines per pane
```
