Output demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1100
Set Height 680
Set Padding 22
Set TypingSpeed 55ms

Hide
Type "export PS1='$ '; export PATH=/Users/antonmoss/code/lazy-tmux/bin:$PATH"
Enter
Type "export TMUX_TMPDIR=/tmp/vhs/tmux; export TMUX=; export LAZY_TMUX_CONFIG=/tmp/vhs/cfg.toml"
Enter
Type "rm -rf /tmp/vhs/tmux /tmp/vhs/data && mkdir -p /tmp/vhs/tmux /tmp/vhs/data"
Enter
Type "alias lt=lazy-tmux"
Enter
Type "tmux new-session -d -s api -n server; tmux send-keys -t api:server top Enter; tmux new-window -t api -n logs; tmux send-keys -t api:logs top Enter"
Enter
Type "tmux new-session -d -s web -n editor; tmux send-keys -t web:editor top Enter; tmux new-window -t web -n shell"
Enter
Type "tmux new-session -d -s notes -n scratch"
Enter
Type "sleep 1; lt save --all >/dev/null 2>&1; clear"
Enter
Sleep 500ms
Show

Type "# Three tmux sessions, saved as snapshots:"
Enter
Sleep 700ms
Type "lt list"
Enter
Sleep 2s

Type "# Now blow away the entire tmux server..."
Enter
Sleep 600ms
Type "tmux kill-server"
Enter
Sleep 1200ms

Type "# ...gone. Restore from the TUI picker:"
Enter
Sleep 700ms
Type "lt picker"
Enter
Sleep 2200ms
Screenshot picker.png

Down
Sleep 900ms
Screenshot picker-nav.png
Enter
Sleep 3s
Screenshot restored.png
Sleep 1200ms
