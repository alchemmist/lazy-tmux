Output out/tui-picker.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 720
Set Padding 20
Set TypingSpeed 55ms

# ---------- hidden setup: a few saved sessions, one asleep ----------
Hide
Type `printf '#!/bin/bash\necho "[api] starting gateway on :8080"\nsleep 100000\n' > /usr/local/bin/gateway; chmod +x /usr/local/bin/gateway`
Enter
Type "rm -rf /root/data; clear"
Enter
Type `tmux new-session -d -s api -n server -x 150 -y 36; tmux send-keys -t api:server 'gateway' Enter`
Enter
Type `tmux new-window -t api -n logs; tmux send-keys -t api:logs 'top' Enter`
Enter
Type `tmux new-session -d -s web -n editor; tmux send-keys -t web:editor "echo 'src/app.tsx  +42 -7'" Enter`
Enter
Type `tmux new-window -t web -n shell`
Enter
Type `tmux new-session -d -s notes -n scratch`
Enter
Type "sleep 1; lazy-tmux save --all >/dev/null 2>&1"
Enter
Type `python3 -c "import json; p='/root/data/sessions/api.json'; d=json.load(open(p)); [pane.update(restore_cmd='gateway') for w in d['windows'] if w['name']=='server' for pane in w['panes']]; json.dump(d,open(p,'w'))"`
Enter
Type "tmux kill-session -t api"
Enter
Type "clear"
Enter
Sleep 500ms
Show

# ---------- open the picker ----------
Type "lazy-tmux picker"
Enter
Sleep 2500ms

# ---------- fuzzy search ----------
Type "web"
Sleep 1800ms
Backspace 3
Sleep 1200ms

# ---------- navigate with ctrl-j / ctrl-k ----------
Ctrl+j
Sleep 500ms
Ctrl+j
Sleep 500ms
Ctrl+k
Sleep 1000ms

# ---------- ? cheat sheet ----------
Type "?"
Sleep 2600ms
Escape
Sleep 900ms

# ---------- / command palette + rename mode ----------
Type "/"
Sleep 1800ms
Type "rename"
Sleep 1200ms
Enter
Sleep 1200ms

# pick the row to rename, confirm with enter, then type the new name
Ctrl+j
Sleep 700ms
Ctrl+j
Sleep 900ms
Enter
Sleep 1200ms
Ctrl+u
Sleep 500ms
Type "todo"
Sleep 900ms
Enter
Sleep 1600ms

# ---------- restore a sleeping session: loading animation ----------
Type "api"
Sleep 1400ms
Enter
Sleep 5500ms

Sleep 1500ms

Hide
Ctrl+b
Sleep 400ms
Type "d"
Sleep 400ms
Type "exit"
Enter
Sleep 300ms
