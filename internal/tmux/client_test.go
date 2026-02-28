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
		{in: "nvim", want: false},
		{in: "", want: false},
	}

	for _, tt := range tests {
		if got := isShellCommand(tt.in); got != tt.want {
			t.Fatalf("isShellCommand(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestNormalizedStartCommand(t *testing.T) {
	if got := normalizedStartCommand("bash", "bash"); got != "" {
		t.Fatalf("shell start command must be dropped, got %q", got)
	}
	if got := normalizedStartCommand("  ", "bash"); got != "" {
		t.Fatalf("empty start command must be dropped, got %q", got)
	}
	if got := normalizedStartCommand("nvim .", "nvim"); got != "nvim ." {
		t.Fatalf("expected start command, got %q", got)
	}
}

func TestFirstPaneInitUsesCleanPathAndCommand(t *testing.T) {
	w := snapshot.Window{
		Panes: []snapshot.Pane{
			{
				CurrentPath:  "/tmp/proj/../proj2",
				StartCommand: "nvim .",
				CurrentCmd:   "nvim",
			},
		},
	}
	path, cmd := firstPaneInit(w)
	if path != "/tmp/proj2" {
		t.Fatalf("unexpected path: %q", path)
	}
	if cmd != "nvim ." {
		t.Fatalf("unexpected command: %q", cmd)
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
					{Index: 0, CurrentPath: "/tmp/proj", StartCommand: "nvim .", CurrentCmd: "nvim"},
					{Index: 1, CurrentPath: "/tmp/proj", StartCommand: "htop", CurrentCmd: "htop"},
				},
			},
			{
				Index:  1,
				Name:   "logs",
				Layout: "tiled",
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/var/log", StartCommand: "tail -f app.log", CurrentCmd: "tail"},
					{Index: 1, CurrentPath: "/var/log", StartCommand: "zsh", CurrentCmd: "zsh"},
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
		"new-session -d -s demo -n editor -c /tmp/proj nvim .",
		"split-window -d -t demo:0 -c /tmp/proj htop",
		"select-layout -t demo:0 even-horizontal",
		"new-window -d -t demo:1 -n logs -c /var/log tail -f app.log",
		"split-window -d -t demo:1 -c /var/log",
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
