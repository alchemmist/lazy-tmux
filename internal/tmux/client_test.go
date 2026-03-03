package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeFakeTmux(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	return path
}

func TestSplitLines(t *testing.T) {
	got := splitLines("  one \n\n two\n\t\nthree  \n")
	if len(got) != 3 || got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Fatalf("unexpected lines: %#v", got)
	}
}

func TestIsShellCommand(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "bash", want: true},
		{in: "-zsh", want: true},
		{in: "/bin/sh", want: true},
		{in: "/bin/zsh -l", want: true},
		{in: "nvim", want: false},
		{in: "", want: false},
	}

	for _, tt := range tests {
		if got := isShellCommand(tt.in); got != tt.want {
			t.Fatalf("isShellCommand(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNormalizedCommand(t *testing.T) {
	if got := normalizedCommand("", "bash"); got != "" {
		t.Fatalf("shell current command must be dropped, got %q", got)
	}
	if got := normalizedCommand("", "  "); got != "" {
		t.Fatalf("empty current command must be dropped, got %q", got)
	}
	if got := normalizedCommand("", "nvim ."); got != "nvim ." {
		t.Fatalf("expected current command, got %q", got)
	}
	if got := normalizedCommand("docker compose up", "bash"); got != "docker compose up" {
		t.Fatalf("expected restore command to win, got %q", got)
	}
	if got := normalizedCommand("\"nvim main.py\"", ""); got != "nvim main.py" {
		t.Fatalf("expected quoted command to be unwrapped, got %q", got)
	}
	if got := normalizedCommand("'ssh laba'", ""); got != "ssh laba" {
		t.Fatalf("expected single-quoted command to be unwrapped, got %q", got)
	}
}

func TestFirstPanePathUsesCleanPath(t *testing.T) {
	w := snapshot.Window{
		Panes: []snapshot.Pane{
			{
				CurrentPath: "/tmp/proj/../proj2",
				CurrentCmd:  "nvim",
				RestoreCmd:  "nvim file.txt",
			},
		},
	}
	path := firstPanePath(w)
	if path != "/tmp/proj2" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestPickForegroundCommandPrefersForegroundMarkedProcess(t *testing.T) {
	lines := []string{
		"1001 S+ -zsh",
		"2002 S docker compose up",
		"2003 R+ ssh user@host",
	}
	got := pickForegroundCommand(lines, 1001)
	if got != "ssh user@host" {
		t.Fatalf("unexpected foreground command: %q", got)
	}
}

func TestPickForegroundCommandFallbackNonShell(t *testing.T) {
	lines := []string{
		"1001 S+ -zsh",
		"2002 S docker compose up",
	}
	got := pickForegroundCommand(lines, 1001)
	if got != "docker compose up" {
		t.Fatalf("unexpected fallback command: %q", got)
	}
}

func TestListSessionsNoServerRunning(t *testing.T) {
	fake := writeFakeTmux(t, `
if [ "$1" = "list-sessions" ]; then
  echo "no server running on /tmp/tmux-1000/default" >&2
  exit 1
fi
exit 0
`)

	c := NewClient(fake)
	got, err := c.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil sessions when no server is running, got %#v", got)
	}
}

func TestRestoreSessionBuildsExpectedTmuxCommands(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmux(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
exit 0
`)
	t.Setenv("TMUX_LOG", logPath)

	c := NewClient(fake)
	s := snapshot.SessionSnapshot{
		SessionName: "demo",
		CurrentWin:  1,
		CurrentPane: 1,
		Windows: []snapshot.Window{
			{
				Index:  0,
				Name:   "editor",
				Layout: "even-horizontal",
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/tmp/proj", CurrentCmd: "nvim ."},
					{Index: 1, CurrentPath: "/tmp/proj", CurrentCmd: "htop"},
				},
			},
			{
				Index:  1,
				Name:   "logs",
				Layout: "tiled",
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/var/log", CurrentCmd: "tail -f app.log"},
					{Index: 1, CurrentPath: "/var/log", CurrentCmd: "zsh"},
				},
			},
		},
	}

	if err := c.RestoreSession(s); err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(b)
	mustContain := []string{
		"has-session -t demo",
		"new-session -d -s demo -n editor -c /tmp/proj",
		"list-windows -t demo -F #{window_index}",
		"split-window -d -t demo:0 -c /tmp/proj",
		"send-keys -t demo:0.0 nvim . C-m",
		"send-keys -t demo:0.1 htop C-m",
		"select-layout -t demo:0 even-horizontal",
		"new-window -d -t demo:1 -n logs -c /var/log",
		"split-window -d -t demo:1 -c /var/log",
		"send-keys -t demo:1.0 tail -f app.log C-m",
		"select-layout -t demo:1 tiled",
		"select-window -t demo:1",
		"select-pane -t demo:1.1",
	}
	for _, needle := range mustContain {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected log to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestRestoreSessionMovesFirstWindowToSnapshotIndex(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmux(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  # tmux default created first window at index 0
  echo "0"
  exit 0
fi
exit 0
`)
	t.Setenv("TMUX_LOG", logPath)

	c := NewClient(fake)
	s := snapshot.SessionSnapshot{
		SessionName: "demo",
		CurrentWin:  5,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 3,
				Name:  "first",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "nvim"}},
			},
			{
				Index: 5,
				Name:  "second",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "htop"}},
			},
		},
	}

	if err := c.RestoreSession(s); err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(b)
	mustContain := []string{
		"new-session -d -s demo -n first -c /tmp",
		"move-window -s demo:0 -t demo:3",
		"new-window -d -t demo:5 -n second -c /tmp",
	}
	for _, needle := range mustContain {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected log to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestRestoreSessionSendsCommandsAfterWindowCreation(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmux(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
exit 0
`)
	t.Setenv("TMUX_LOG", logPath)

	c := NewClient(fake)
	s := snapshot.SessionSnapshot{
		SessionName: "demo",
		CurrentWin:  1,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 0,
				Name:  "ok",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "nvim"}},
			},
			{
				Index: 1,
				Name:  "commands",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "echo ok"}},
			},
		},
	}

	if err := c.RestoreSession(s); err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "new-window -d -t demo:1 -n commands -c /tmp") {
		t.Fatalf("expected new-window without inline command, got:\n%s", out)
	}
	if !strings.Contains(out, "send-keys -t demo:1.0 echo ok C-m") {
		t.Fatalf("expected command via send-keys, got:\n%s", out)
	}
}

func TestRestoreSessionFallsBackWithoutPathWhenPathFails(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmux(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
if [ "$1" = "new-session" ] || [ "$1" = "new-window" ]; then
  case "$*" in
    *"-c /bad/path"*)
      # Simulate bad cwd even without command.
      exit 1
      ;;
  esac
fi
exit 0
`)
	t.Setenv("TMUX_LOG", logPath)

	c := NewClient(fake)
	s := snapshot.SessionSnapshot{
		SessionName: "demo",
		CurrentWin:  1,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 0,
				Name:  "ok",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/bad/path", CurrentCmd: "nvim"}},
			},
			{
				Index: 1,
				Name:  "fallback",
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/bad/path", CurrentCmd: "htop"}},
			},
		},
	}

	if err := c.RestoreSession(s); err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "new-window -d -t demo:1 -n fallback -c /bad/path") {
		t.Fatalf("expected initial attempt with bad path, got:\n%s", out)
	}
	if !strings.Contains(out, "new-window -d -t demo:1 -n fallback") {
		t.Fatalf("expected final fallback without path, got:\n%s", out)
	}
	if !strings.Contains(out, "new-session -d -s demo -n ok") {
		t.Fatalf("expected new-session fallback without bad path, got:\n%s", out)
	}
}

func TestRestoreSessionReplaysScrollbackToPaneTTY(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	var gotWritten string
	origWriter := paneTTYWriter
	paneTTYWriter = func(path, content string) error {
		gotWritten = content
		return nil
	}
	t.Cleanup(func() { paneTTYWriter = origWriter })

	fake := writeFakeTmux(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
if [ "$1" = "display-message" ] && [ "$5" = "#{pane_tty}" ]; then
  echo "/dev/pts/42"
  exit 0
fi
exit 0
`)
	t.Setenv("TMUX_LOG", logPath)

	c := NewClient(fake)
	s := snapshot.SessionSnapshot{
		SessionName: "demo",
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 0,
				Name:  "shell",
				Panes: []snapshot.Pane{
					{
						Index:      0,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{
							Content: "echo old\nold output\n",
						},
					},
				},
			},
		},
	}

	if err := c.RestoreSession(s); err != nil {
		t.Fatalf("RestoreSession error: %v", err)
	}

	if !strings.Contains(gotWritten, "old output") {
		t.Fatalf("expected scrollback replay in pane tty, got:\n%s", gotWritten)
	}
}
