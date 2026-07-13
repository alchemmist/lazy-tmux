Output out/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 720
Set Padding 20
Set TypingSpeed 60ms

# ---------- hidden setup: build a session, save it ----------
Hide
Type "export PS1='\[\e[38;5;39m\]demo\[\e[0m\]:\w$ '; export LAZY_TMUX_CONFIG=/root/cfg.toml; export TMUX_TMPDIR=/root/tmuxtmp; export TERM=xterm-256color"
Enter
Type "rm -rf /root/data /root/tmuxtmp; mkdir -p /root/data /root/tmuxtmp; clear"
Enter
Type "tmux new-session -d -s dev -n shell -x 140 -y 34"
Enter
Type `tmux send-keys -t dev:shell "echo 'BUILD: compiling modules...'; seq 1 12; echo 'BUILD: done - server listening on :8080'" Enter`
Enter
Type `tmux new-window -t dev -n monitor; tmux send-keys -t dev:monitor 'top' Enter`
Enter
Type "tmux select-window -t dev:shell"
Enter
Type `tmux bind-key -n M-p display-popup -E -w 70% -h 55% 'lazy-tmux picker'`
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
Alt+p
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
Sleep 4500ms
Enter
Sleep 4500ms

# ---------- nothing was lost ----------
Sleep 1500ms
Type "# the shell's output history is right where we left it"
Sleep 600ms
Enter
Sleep 1800ms
Type "# and top is still running next door"
Sleep 600ms
Enter
Sleep 1200ms
Ctrl+b
Sleep 400ms
Type "n"
Sleep 3500ms
