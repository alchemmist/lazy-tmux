Output docs/public/assets/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 720
Set Padding 20
Set TypingSpeed 60ms

# ---------- hidden setup: build a session, save it ----------
Hide
Type `W="$HOME/.cache/lazy-tmux-demo"; export PATH="$LT_BIN_DIR:$PATH" TERM=xterm-256color LANG=C.UTF-8 LC_ALL=C.UTF-8 SHELL=/bin/bash; unset TMUX; export LAZY_TMUX_CONFIG="$W/cfg.toml" TMUX_TMPDIR="$W/tmux" HOME="$W/home"`
Enter
Type `rm -rf "$W"; mkdir -p "$W/data" "$W/tmux" "$W/home"; printf 'data_dir="%s/data"\n[scrollback]\nenabled=true\nlines=5000\n' "$W" > "$W/cfg.toml"; printf 'PS1="demo:\\w\\$ "\n' > "$W/home/.bash_profile"; cp "$W/home/.bash_profile" "$W/home/.bashrc"; printf 'set -g status-right " %%H:%%M %%d-%%b "\nset -sg escape-time 20\n' > "$W/home/.tmux.conf"; export PS1='\[\e[38;5;39m\]demo\[\e[0m\]:\w$ '; cd "$HOME"; clear`
Enter
Type "tmux new-session -d -s dev -n shell -x 140 -y 34"
Enter
Type `tmux send-keys -t dev:shell "echo 'BUILD: compiling modules...'; seq 1 12; echo 'BUILD: done - server listening on :8080'" Enter`
Enter
Type `tmux new-window -t dev -n monitor; tmux send-keys -t dev:monitor 'top' Enter`
Enter
Type "tmux select-window -t dev:shell"
Enter
Type `tmux bind-key f display-popup -E -w 70% -h 55% "$LT_BIN_DIR/lazy-tmux picker"`
Enter
Type "sleep 1; lazy-tmux save --all >/dev/null 2>&1; clear"
Enter
Sleep 800ms
Type "tmux attach -t dev"
Enter
Sleep 2000ms
Show

# ---------- inside the session ----------
Sleep 1500ms
Type "# a shell window with real build output above"
Sleep 600ms
Enter
Sleep 1800ms

Type "# top is running in the next window - let's switch"
Sleep 600ms
Enter
Sleep 1200ms
Ctrl+b
Sleep 400ms
Type "n"
Sleep 3000ms
Ctrl+b
Sleep 400ms
Type "p"
Sleep 1800ms

Type "# open the lazy-tmux picker in a floating window"
Sleep 600ms
Enter
Sleep 800ms
Ctrl+b
Sleep 400ms
Type "f"
Sleep 2200ms
Ctrl+j
Sleep 1600ms
Escape
Sleep 1200ms

# ---------- detach and destroy ----------
Ctrl+b
Sleep 400ms
Type "d"
Sleep 1600ms
Type "# detached. the snapshot is saved - sessions survive:"
Enter
Sleep 800ms
Type "tmux ls"
Enter
Sleep 2200ms
Type "# now blow away the entire tmux server..."
Enter
Sleep 700ms
Type "tmux kill-server"
Enter
Sleep 1200ms
Type "tmux ls"
Enter
Sleep 2000ms

# ---------- restore ----------
Type "# ...gone. restore it all with the picker:"
Enter
Sleep 700ms
Type "lazy-tmux picker"
Enter
Sleep 1200ms
Type "moni"
Sleep 1200ms
Enter
Sleep 4500ms

# ---------- nothing was lost ----------
Sleep 1500ms
Ctrl+b
Sleep 400ms
Type "n"
Sleep 100ms
Type "# top is still running"
Enter
Sleep 1200ms
Type "# and the shell's output history is right where we left it"
Enter
Sleep 1800ms
Sleep 3500ms

Hide
Ctrl+b
Sleep 500ms
Type "d"
Sleep 400ms
Type "exit"
Enter
Sleep 300ms
