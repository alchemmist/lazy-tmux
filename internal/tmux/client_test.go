package tmux

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// fakeRunner records all tmux commands and returns configurable responses.
type fakeRunner struct {
	commands []string          // recorded command sequences
	outputs  map[string]string // prefix → stdout response
	errors   map[string]string // prefix → error message (if non-empty)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]string),
	}
}

func (f *fakeRunner) run(args ...string) commandResult {
	f.commands = append(f.commands, strings.Join(args, " "))

	for prefix, out := range f.outputs {
		if strings.Join(args, " ")[:len(prefix)] == prefix {
			if err := f.errors[prefix]; err != "" {
				return commandResult{"", fmt.Errorf("%s", err)}
			}

			return commandResult{out, nil}
		}
	}

	return commandResult{"", nil}
}

func (f *fakeRunner) setResponse(prefix, stdout, errMsg string) {
	f.outputs[prefix] = stdout
	if errMsg != "" {
		f.errors[prefix] = errMsg
	}
}

func TestRestoreSessionCommandSequence(t *testing.T) {
	r := newFakeRunner()

	// Session does not exist
	r.setResponse("tmux has-session", "", "no server")
	// Meta: current window and pane
	r.setResponse("tmux display-message", "2\x1f1", "")
	// List windows
	r.setResponse("tmux list-windows", "0\x1fmain\x1feven-horizontal\x1f0\n1\x1flogs\x1feven-horizontal\x1f1\n", "")
	// List panes for window 0
	r.setResponse("tmux list-panes", "0\x1f/tmp\x1fbash\x1f1\x1f1001\x1f/dev/pts/0\n", "")
	// List panes for window 1
	r.setResponse("tmux list-panes -t", "0\x1f/var\x1fzsh\x1f0\x1f1002\x1f/dev/pts/1\n", "")

	client := NewClientWithRunner("tmux", r)

	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "test-session",
		CurrentWin:  2,
		CurrentPane: 1,
		Windows: []snapshot.Window{
			{
				Index:    0,
				Name:     "main",
				Layout:   "even-horizontal",
				IsActive: false,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/tmp", CurrentCmd: "bash", RestoreCmd: "nvim", IsActive: true},
				},
			},
			{
				Index:    1,
				Name:     "logs",
				Layout:   "even-horizontal",
				IsActive: true,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/var", CurrentCmd: "zsh", RestoreCmd: "docker compose up", IsActive: false},
				},
			},
		},
	}

	err := client.RestoreSession(snap)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}

	// Verify the command sequence
	if len(r.commands) == 0 {
		t.Fatal("no commands were recorded")
	}

	// First command should be has-session
	if !strings.Contains(r.commands[0], "has-session") {
		t.Fatalf("expected has-session first, got: %s", r.commands[0])
	}

	// Verify new-session was called
	hasNewSession := false
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "new-session") {
			hasNewSession = true
			break
		}
	}

	if !hasNewSession {
		t.Fatal("expected new-session command, got: " + fmt.Sprint(r.commands))
	}

	// Verify select-window was called with correct target
	hasSelectWindow := false
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "select-window") && strings.Contains(cmd, "=test-session:2") {
			hasSelectWindow = true
			break
		}
	}

	if !hasSelectWindow {
		t.Fatal("expected select-window for =test-session:2, got: " + fmt.Sprint(r.commands))
	}
}

func TestListSessionsWithFakeRunner(t *testing.T) {
	r := newFakeRunner()
	r.setResponse("tmux list-sessions", "dev\nprod\nstaging\n", "")

	client := NewClientWithRunner("tmux", r)

	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	expected := []string{"dev", "prod", "staging"}
	if len(sessions) != len(expected) {
		t.Fatalf("expected %d sessions, got %d", len(expected), len(sessions))
	}

	for i, name := range expected {
		if sessions[i] != name {
			t.Fatalf("session[%d] = %q, want %q", i, sessions[i], name)
		}
	}
}

func TestNewWindowWithFakeRunner(t *testing.T) {
	r := newFakeRunner()
	r.setResponse("tmux new-window", "", "")

	client := NewClientWithRunner("tmux", r)

	err := client.NewWindow("my-session", "editor")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	// Verify the command was issued correctly
	found := false
	for _, cmd := range r.commands {
		if strings.Contains(cmd, "new-window") &&
			strings.Contains(cmd, "my-session") &&
			strings.Contains(cmd, "editor") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected new-window for my-session/editor, got: %v", r.commands)
	}
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
