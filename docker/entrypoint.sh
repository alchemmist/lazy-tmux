#!/usr/bin/env bash
set -e

SESSION="demo"

# tmux config
cat <<'EOF' >/root/.tmux.conf
run-shell -b 'lazy-tmux daemon --interval 3m --scrollback --scrollback-lines 5000 || true'
bind-key f display-popup -w 65% -h 75% -E 'lazy-tmux picker'
bind-key C-s run-shell 'lazy-tmux save --all --scrollback && tmux display-message "All sessions saved successfully!"'
EOF

# zsh config
cat <<'EOF' >/root/.zshrc
lazy_tmux_help() {
  echo "=== lazy-tmux detailed guide ==="
  echo
  echo "[Picker]"
  echo "Prefix + f → open picker"
  echo "Enter      → restore selected session/window"
  echo "Ctrl+j/k   → navigate"
  echo "Typing     → fuzzy search"
  echo "Alt+n      → create session"
  echo "Alt-w      → wake up session"
  echo "Alt-s      → sleep session"
  echo
  echo "[Core commands]"
  echo "lazy-tmux picker"
  echo "lazy-tmux save --all --scrollback"
  echo "lazy-tmux restore --session NAME"
  echo "lazy-tmux list"
  echo "lazy-tmux daemon"
  echo
  echo "[Docs]"
  echo "https://lazy-tmux.xyz"
}

_lazy_tmux_welcome() {
  echo ""
  echo "Welcome to the lazy-tmux sandbox"
  echo ""
  echo "This environment is preconfigured with tmux + lazy-tmux."
  echo ""
  echo "Quick start:"
  echo "  Prefix + f        → open picker"
  echo ""
  echo "For detailed help run:"
  echo "  lazy_tmux_help"
  echo ""
  echo "Documentation:"
  echo "  https://lazy-tmux.xyz"
  echo ""
}
EOF

zsh

tmux new-session -d -s "$SESSION"

# lazy-tmux save --all --scrollback

tmux attach -t "$SESSION"
