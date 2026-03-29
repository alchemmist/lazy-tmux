package tmux

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

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
	win := snapshot.Window{
		Panes: []snapshot.Pane{
			{
				CurrentPath: "/tmp/proj/../proj2",
				CurrentCmd:  "nvim",
				RestoreCmd:  "nvim file.txt",
			},
		},
	}

	path := firstPanePath(win)
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
