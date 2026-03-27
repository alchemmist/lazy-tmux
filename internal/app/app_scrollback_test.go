package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestIsShellCommandName(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "bash", want: true},
		{in: "-zsh", want: true},
		{in: "/bin/sh -l", want: true},
		{in: "fish", want: true},
		{in: "nvim", want: false},
		{in: "", want: false},
	}
	for _, tt := range tests {
		if got := isShellCommandName(tt.in); got != tt.want {
			t.Fatalf("isShellCommandName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCaptureShellScrollbackSkipsNonShellAndRestoreCmd(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmuxForApp(t, `
echo "$*" >> "$TMUX_LOG"
exit 0
`)

	t.Setenv("TMUX_LOG", logPath)

	a := &App{
		cfg:  config.Config{Scrollback: config.ScrollbackConfig{Enabled: true, Lines: 10}},
		tmux: tmux.NewClient(fake),
	}

	snap := snapshot.SessionSnapshot{
		SessionName: "demo",
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "nvim"},
					{Index: 1, CurrentCmd: "zsh", RestoreCmd: "nvim main.go"},
				},
			},
		},
	}

	a.captureShellScrollback(&snap)

	if snap.Windows[0].Panes[0].Scrollback != nil {
		t.Fatal("expected non-shell pane to skip scrollback capture")
	}

	if snap.Windows[0].Panes[1].Scrollback != nil {
		t.Fatal("expected restore cmd pane to skip scrollback capture")
	}

	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("tmux capture-pane must not be called for skipped panes")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
}

func TestCaptureShellScrollbackSkipsEmptyOutput(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "capture-pane" ]; then
  printf "\n\n"
  exit 0
fi
exit 0
`)

	a := &App{
		cfg:  config.Config{Scrollback: config.ScrollbackConfig{Enabled: true, Lines: 10}},
		tmux: tmux.NewClient(fake),
	}

	snap := snapshot.SessionSnapshot{
		SessionName: "demo",
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "bash"},
				},
			},
		},
	}

	a.captureShellScrollback(&snap)

	if snap.Windows[0].Panes[0].Scrollback != nil {
		t.Fatalf("expected empty scrollback to be skipped, got: %+v", snap.Windows[0].Panes[0].Scrollback)
	}
}
