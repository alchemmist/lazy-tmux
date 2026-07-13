# Demo recording

`demo.gif` (used in the project README and the docs landing page) is generated
from `demo.tape` with [VHS](https://github.com/charmbracelet/vhs), running
inside a throwaway container so the recording never touches a real tmux server
or your own snapshots.

The tape walks through the core promise of lazy-tmux: start inside a tmux
session with `top` and a shell, open the picker in a floating window, detach,
`tmux kill-server`, then restore everything from the picker — output history and
running processes intact.

## Regenerate

From the repository root:

```sh
# 1. cross-compile a static Linux binary for the container
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o docs/tapes/lazy-tmux ./cmd/lazy-tmux

# 2. build the recording image (VHS + tmux + procps + lazy-tmux)
podman build -t lazy-tmux-vhs docs/tapes

# 3. record
mkdir -p docs/tapes/out
podman run --rm \
  -v "$PWD/docs/tapes/cfg.toml:/root/cfg.toml:ro" \
  -v "$PWD/docs/tapes/demo.tape:/root/demo.tape:ro" \
  -v "$PWD/docs/tapes/out:/root/out" \
  lazy-tmux-vhs /root/demo.tape

# 4. publish
cp docs/tapes/out/demo.gif docs/public/assets/demo.gif
```

Use `GOARCH=amd64` on an x86 host. The `lazy-tmux` binary and `out/` directory
are build artifacts and are not committed.
