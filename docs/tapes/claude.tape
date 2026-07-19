Output out/claude.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 720
Set Padding 20
Set TypingSpeed 55ms

# ---------- hidden setup: several claude panes, one per state ----------
Hide
Type "cp $(command -v cat) /usr/local/bin/claude"
Enter
Type "rm -rf /root/data; mkdir -p /root/api /root/web /root/worker /root/notes; clear"
Enter
Type `tmux new-session -d -s dev -n api -c /root/api -x 150 -y 36; tmux send-keys -t dev:api 'claude' Enter`
Enter
Type `tmux new-window -t dev -n web -c /root/web; tmux send-keys -t dev:web 'claude' Enter`
Enter
Type `tmux new-window -t dev -n worker -c /root/worker; tmux send-keys -t dev:worker 'claude' Enter`
Enter
Type `tmux new-window -t dev -n notes -c /root/notes; tmux send-keys -t dev:notes 'claude' Enter`
Enter
Type `tmux new-window -t dev -n shell`
Enter
Type "tmux select-window -t dev:api"
Enter
Type "sleep 1; lazy-tmux save --all >/dev/null 2>&1"
Enter
Type "mkdir -p /root/data/claude-status"
Enter
Type `printf '{"state":"working","cwd":"/root/api"}' > /root/data/claude-status/-root-api.json`
Enter
Type `printf '{"state":"awaiting_decision","cwd":"/root/web"}' > /root/data/claude-status/-root-web.json`
Enter
Type `printf '{"state":"awaiting_input","cwd":"/root/worker"}' > /root/data/claude-status/-root-worker.json`
Enter
Type `printf '{"state":"idle","cwd":"/root/notes"}' > /root/data/claude-status/-root-notes.json`
Enter
Type "clear"
Enter
Sleep 500ms
Show

# ---------- the picker shows a live status glyph per claude window ----------
Type "lazy-tmux picker"
Enter
Sleep 7000ms
Ctrl+j
Sleep 1200ms
Ctrl+j
Sleep 1200ms
Ctrl+j
Sleep 2500ms

Hide
Escape
Sleep 200ms
Type "exit"
Enter
Sleep 300ms
