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

func TestExecutableName(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{cmd: "bash", want: "bash"},
		{cmd: "-zsh", want: "zsh"},
		{cmd: "/bin/bash -l", want: "bash"},
		{cmd: "/usr/bin/nvim main.go", want: "nvim"},
		{cmd: "", want: ""},
		{cmd: "   ", want: ""},
	}

	for _, tt := range tests {
		if got := executableName(tt.cmd); got != tt.want {
			t.Fatalf("executableName(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestSanitizeCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{cmd: `"nvim main.py"`, want: "nvim main.py"},
		{cmd: `'ssh user@host'`, want: "ssh user@host"},
		{cmd: `'single'`, want: "single"},
		{cmd: `"double"`, want: "double"},
		{cmd: `plain command`, want: "plain command"},
		{cmd: `  spaces  `, want: "spaces"},
		{cmd: `"mismatched'`, want: `"mismatched'`},
		{cmd: `''`, want: ""},
		{cmd: `""`, want: ""},
	}

	for _, tt := range tests {
		if got := sanitizeCommand(tt.cmd); got != tt.want {
			t.Fatalf("sanitizeCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestStripOptionPair(t *testing.T) {
	tests := []struct {
		args []string
		opt  string
		want []string
	}{
		{
			args: []string{"-c", "/tmp", "-n", "name", "rest"},
			opt:  "-c",
			want: []string{"-n", "name", "rest"},
		},
		{
			args: []string{"-n", "name"},
			opt:  "-n",
			want: []string{},
		},
		{
			args: []string{"a", "b", "c"},
			opt:  "-x",
			want: []string{"a", "b", "c"},
		},
		{
			args: []string{},
			opt:  "-c",
			want: []string{},
		},
	}

	for _, testCase := range tests {
		got := stripOptionPair(testCase.args, testCase.opt)
		if len(got) != len(testCase.want) {
			t.Fatalf(
				"stripOptionPair(%v, %q) length mismatch: got %d, want %d",
				testCase.args,
				testCase.opt,
				len(got),
				len(testCase.want),
			)
		}

		for idx, val := range got {
			if val != testCase.want[idx] {
				t.Fatalf(
					"stripOptionPair(%v, %q)[%d] = %q, want %q",
					testCase.args,
					testCase.opt,
					idx,
					val,
					testCase.want[idx],
				)
			}
		}
	}
}

func TestSessionTarget(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "demo", want: "=demo"},
		{name: "=demo", want: "=demo"},
		{name: " demo ", want: "=demo"},
		{name: "", want: "="},
	}

	for _, tt := range tests {
		if got := sessionTarget(tt.name); got != tt.want {
			t.Fatalf("sessionTarget(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSessionWindowTarget(t *testing.T) {
	tests := []struct {
		name        string
		windowIndex int
		want        string
	}{
		{name: "demo", windowIndex: 0, want: "=demo:0"},
		{name: "test", windowIndex: 5, want: "=test:5"},
		{name: "=session", windowIndex: 1, want: "=session:1"},
	}

	for _, testCase := range tests {
		if got := sessionWindowTarget(testCase.name, testCase.windowIndex); got != testCase.want {
			t.Fatalf(
				"sessionWindowTarget(%q, %d) = %q, want %q",
				testCase.name,
				testCase.windowIndex,
				got,
				testCase.want,
			)
		}
	}
}

func TestSessionPaneTarget(t *testing.T) {
	tests := []struct {
		name        string
		windowIndex int
		paneIndex   int
		want        string
	}{
		{name: "demo", windowIndex: 0, paneIndex: 0, want: "=demo:0.0"},
		{name: "test", windowIndex: 2, paneIndex: 1, want: "=test:2.1"},
		{name: "=session", windowIndex: 0, paneIndex: 3, want: "=session:0.3"},
	}

	for _, testCase := range tests {
		got := sessionPaneTarget(testCase.name, testCase.windowIndex, testCase.paneIndex)
		if got != testCase.want {
			t.Fatalf(
				"sessionPaneTarget(%q, %d, %d) = %q, want %q",
				testCase.name,
				testCase.windowIndex,
				testCase.paneIndex,
				got,
				testCase.want,
			)
		}
	}
}

func TestParsePSLineHelper(t *testing.T) {
	tests := []struct {
		line     string
		wantPID  int
		wantStat string
		wantCmd  string
		wantOK   bool
	}{
		{
			line:     "1234 S- bash",
			wantPID:  1234,
			wantStat: "S-",
			wantCmd:  "bash",
			wantOK:   true,
		},
		{
			line:     "2002 R+ docker compose up",
			wantPID:  2002,
			wantStat: "R+",
			wantCmd:  "docker compose up",
			wantOK:   true,
		},
		{
			line:   "invalid",
			wantOK: false,
		},
		{
			line:   "",
			wantOK: false,
		},
	}

	for _, testCase := range tests {
		pid, stat, cmd, ok := parsePSLine(testCase.line)
		if ok != testCase.wantOK {
			t.Fatalf("parsePSLine(%q) ok = %v, want %v", testCase.line, ok, testCase.wantOK)
		}

		if !ok {
			continue
		}

		if pid != testCase.wantPID {
			t.Fatalf("parsePSLine(%q) pid = %d, want %d", testCase.line, pid, testCase.wantPID)
		}

		if stat != testCase.wantStat {
			t.Fatalf("parsePSLine(%q) stat = %q, want %q", testCase.line, stat, testCase.wantStat)
		}

		if cmd != testCase.wantCmd {
			t.Fatalf("parsePSLine(%q) cmd = %q, want %q", testCase.line, cmd, testCase.wantCmd)
		}
	}
}
