# Demo recordings

The GIFs used across the docs and README are generated from the `.tape` files in
this directory with [VHS](https://github.com/charmbracelet/vhs).

Each tape records to `docs/public/assets/<name>.gif`:

- `demo.tape` → `demo.gif` — save, `tmux kill-server`, restore from the picker.
- `tui-picker.tape` → `tui-picker.gif` — fuzzy search, the `/` command palette,
  the `?` cheat sheet, and the restore loading animation.
- `claude.tape` → `claude.gif` — the Claude Code status glyphs in the picker.

## Regenerate

```sh
make gifs
```

This builds a prepared, throwaway container (`docs/tapes/Dockerfile`: VHS + tmux
+ the `lazy-tmux` built from the current working tree, with a quiet prompt, a
UTF-8 locale and scrollback enabled), then records every `*.tape` in this
directory inside it, dropping each GIF straight into `docs/public/assets/`.

Recording runs entirely in the container, so it never touches your real tmux
server or your own snapshots. Requires `podman` and Go on the host; the
cross-compiled `lazy-tmux` binary here is a build artifact and is not committed.
