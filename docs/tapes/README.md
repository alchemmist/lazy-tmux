# Demo recording

`demo.gif` (used in the project README and the docs landing page) is generated
from `demo.tape` with [VHS](https://github.com/charmbracelet/vhs).

The tape walks through the core promise of lazy-tmux: start inside a tmux
session with `top` and a shell, open the picker in a floating window, detach,
`tmux kill-server`, then restore everything from the picker — output history and
running processes intact.

## Regenerate

```sh
make demo-gif
```

This builds `bin/lazy-tmux` from the current working tree and runs VHS on
`demo.tape`, writing the result straight to `docs/public/assets/demo.gif`.

Requires `vhs`, `ttyd` and `tmux` on your PATH (`brew install vhs ttyd tmux`).
The recording runs an isolated tmux server (`TMUX_TMPDIR`) against a throwaway
data dir under `~/.cache/lazy-tmux-demo`, so it never touches your real tmux
sessions or snapshots.
