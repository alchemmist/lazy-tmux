package tmux

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// pollRunner is a fake tmux runner that reports a pane running the shell until
// the configured poll, then reports the restored command. It lets us drive
// waitForRestoredCommands deterministically without a real tmux server.
type pollRunner struct {
	calls    int
	settleOn int    // 1-based poll at which the command finally appears
	before   string // pane_current_command before it settles
	after    string // pane_current_command once it settles
}

func (r *pollRunner) runCommand(args ...string) commandResult {
	if len(args) > 1 && args[1] == "list-panes" {
		r.calls++

		cmd := r.before
		if r.calls >= r.settleOn {
			cmd = r.after
		}

		return commandResult{stdout: "1 0 " + cmd + "\n"}
	}

	return commandResult{}
}

func waitTestWindows() []snapshot.Window {
	return []snapshot.Window{
		{Index: 1, Panes: []snapshot.Pane{{Index: 0, RestoreCmd: "cat"}}},
	}
}

func TestWaitForRestoredCommandsBlocksUntilStarted(t *testing.T) {
	runner := &pollRunner{settleOn: 3, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(2 * time.Second)

	client.waitForRestoredCommands("sess", waitTestWindows())

	// It must keep polling while the pane still shows the shell, only returning
	// once the restored command actually appears.
	if runner.calls < 3 {
		t.Fatalf("expected to poll until command started, polled %d times", runner.calls)
	}
}

func TestWaitForRestoredCommandsRespectsTimeout(t *testing.T) {
	// The command never appears: the wait must give up at the deadline, not hang.
	runner := &pollRunner{settleOn: 1 << 30, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(150 * time.Millisecond)

	start := time.Now()
	client.waitForRestoredCommands("sess", waitTestWindows())

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("wait ignored its timeout, took %v", elapsed)
	}

	if runner.calls == 0 {
		t.Fatal("expected at least one poll before giving up")
	}
}

func TestWaitForRestoredCommandsDisabled(t *testing.T) {
	runner := &pollRunner{settleOn: 1, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(0)

	client.waitForRestoredCommands("sess", waitTestWindows())

	if runner.calls != 0 {
		t.Fatalf("a disabled timeout must not poll, polled %d times", runner.calls)
	}
}

// The field separator must be printable ASCII: tmux sanitizes non-printable
// bytes in -F output (the previous 0x1f became "_" on some builds, collapsing
// every field into one and dropping all windows — see the version-matrix bug).
func TestFieldSepIsPrintableASCII(t *testing.T) {
	if len(fieldSep) == 0 {
		t.Fatal("fieldSep must not be empty")
	}

	for _, r := range fieldSep {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("fieldSep must be printable ASCII, got %q", fieldSep)
		}
	}
}

// The free-form fields (window name, pane current path) are placed last in the
// -F format strings, so splitFieldsN must keep a trailing field intact even when
// it contains the separator.
func TestSplitFieldsNKeepsTrailingFieldIntact(t *testing.T) {
	line := "3" + fieldSep + "bb,80x24" + fieldSep + "1" + fieldSep + "weird" + fieldSep + "name"
	got := splitFieldsN(line, 4)

	want := []string{"3", "bb,80x24", "1", "weird" + fieldSep + "name"}
	if len(got) != len(want) {
		t.Fatalf("expected %d fields, got %d: %q", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCurrentWindowPane(t *testing.T) {
	// Active window's index and active pane are authoritative, even when the
	// base index is 1 (regression guard: current window must not collapse to 0).
	windows := []snapshot.Window{
		{Index: 1, IsActive: false, ActivePane: 0},
		{Index: 2, IsActive: true, ActivePane: 3},
	}

	win, pane := currentWindowPane(windows)
	if win != 2 || pane != 3 {
		t.Fatalf("expected active window 2 pane 3, got %d/%d", win, pane)
	}

	// No active flag -> fall back to the first window.
	win, pane = currentWindowPane([]snapshot.Window{{Index: 5, ActivePane: 1}})
	if win != 5 || pane != 1 {
		t.Fatalf("expected fallback to first window 5/1, got %d/%d", win, pane)
	}

	// Empty -> zero values.
	win, pane = currentWindowPane(nil)
	if win != 0 || pane != 0 {
		t.Fatalf("expected 0/0 for no windows, got %d/%d", win, pane)
	}
}

func TestExpectedPaneCommands(t *testing.T) {
	windows := []snapshot.Window{
		{
			Index: 1,
			Panes: []snapshot.Pane{
				{Index: 0, RestoreCmd: "nvim main.go"}, // real command -> nvim
				{Index: 1, RestoreCmd: "zsh"},          // shell only -> skipped
			},
		},
		{
			Index: 2,
			Panes: []snapshot.Pane{
				{Index: 0, CurrentCmd: "htop -d 5"}, // falls back to current command
			},
		},
	}

	// No allowlist configured -> every real command is expected.
	got := NewClient("tmux").expectedPaneCommands(windows)

	want := map[string]string{"1.0": "nvim", "2.0": "htop"}
	if len(got) != len(want) {
		t.Fatalf("expectedPaneCommands size: got %#v want %#v", got, want)
	}

	for key, exe := range want {
		if got[key] != exe {
			t.Fatalf("expectedPaneCommands[%q]=%q want %q (full %#v)", key, got[key], exe, got)
		}
	}
}

func TestRestoreAllowlist(t *testing.T) {
	client := NewClient("tmux")

	// No allowlist configured: everything is allowed.
	if !client.commandAllowed("nvim") || !client.commandAllowed("rm") {
		t.Fatal("with no allowlist, all commands must be allowed")
	}

	// Configured allowlist: only listed executables (path entries normalized).
	client.SetRestoreAllowlist([]string{"nvim", "/usr/bin/htop", "  tmux  "})

	for _, allowed := range []string{"nvim", "htop", "tmux"} {
		if !client.commandAllowed(allowed) {
			t.Fatalf("%q should be allowed", allowed)
		}
	}

	for _, blocked := range []string{"rm", "bash", "node"} {
		if client.commandAllowed(blocked) {
			t.Fatalf("%q should be blocked", blocked)
		}
	}

	// Empty (but configured) allowlist blocks everything.
	client.SetRestoreAllowlist([]string{})

	if client.commandAllowed("nvim") {
		t.Fatal("an empty allowlist must block all commands")
	}

	// Clearing with nil restores allow-all behavior.
	client.SetRestoreAllowlist(nil)

	if !client.commandAllowed("nvim") {
		t.Fatal("a nil allowlist must allow all commands again")
	}
}

func TestRestoreDenylist(t *testing.T) {
	client := NewClient("tmux")

	// No denylist configured: everything is allowed.
	if !client.commandAllowed("node") {
		t.Fatal("with no denylist, all commands must be allowed")
	}

	// Configured denylist: listed executables are blocked (path entries normalized).
	client.SetRestoreDenylist([]string{"node", "/usr/bin/htop", "  npm  "})

	for _, blocked := range []string{"node", "htop", "npm"} {
		if client.commandAllowed(blocked) {
			t.Fatalf("%q should be blocked", blocked)
		}
	}

	for _, allowed := range []string{"nvim", "vim", "less"} {
		if !client.commandAllowed(allowed) {
			t.Fatalf("%q should be allowed", allowed)
		}
	}

	// Clearing with nil (or empty) restores allow-all behavior.
	client.SetRestoreDenylist(nil)

	if !client.commandAllowed("node") {
		t.Fatal("a nil denylist must allow all commands again")
	}
}

func TestRestoreDenylistWinsOverAllowlist(t *testing.T) {
	client := NewClient("tmux")

	// A command in both lists is blocked: the denylist takes precedence.
	client.SetRestoreAllowlist([]string{"nvim", "node"})
	client.SetRestoreDenylist([]string{"node"})

	if !client.commandAllowed("nvim") {
		t.Fatal("nvim is allowed and not denied -> should be restored")
	}

	if client.commandAllowed("node") {
		t.Fatal("node is denied -> must be blocked even though the allowlist permits it")
	}

	// A command that is neither allowed nor denied stays blocked by the allowlist.
	if client.commandAllowed("htop") {
		t.Fatal("htop is not in the allowlist -> should stay blocked")
	}
}

func TestExpectedPaneCommandsRespectsAllowlist(t *testing.T) {
	windows := []snapshot.Window{
		{
			Index: 1,
			Panes: []snapshot.Pane{
				{Index: 0, RestoreCmd: "nvim"},
				{Index: 1, RestoreCmd: "htop"},
			},
		},
	}

	client := NewClient("tmux")
	client.SetRestoreAllowlist([]string{"nvim"})

	got := client.expectedPaneCommands(windows)

	if len(got) != 1 || got["1.0"] != "nvim" {
		t.Fatalf("allowlist should keep only nvim, got %#v", got)
	}
}

func TestSessionTargets(t *testing.T) {
	if got := sessionTarget("foo"); got != "=foo" {
		t.Fatalf("sessionTarget: got %q", got)
	}

	if got := sessionTarget("=foo"); got != "=foo" {
		t.Fatalf("sessionTarget already-prefixed: got %q", got)
	}

	if got := sessionWindowTarget("foo", 2); got != "=foo:2" {
		t.Fatalf("sessionWindowTarget: got %q", got)
	}

	if got := PaneTarget("foo", 2, 3); got != "=foo:2.3" {
		t.Fatalf("PaneTarget: got %q", got)
	}
}

func TestSanitizeCommand(t *testing.T) {
	cases := map[string]string{
		`"vim"`:    "vim",
		`'vim'`:    "vim",
		`  vim  `:  "vim",
		`vim file`: "vim file",
	}

	for in, want := range cases {
		if got := sanitizeCommand(in); got != want {
			t.Fatalf("sanitizeCommand(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsShellCommandAndExecutableName(t *testing.T) {
	for _, sh := range []string{"bash", "/bin/zsh", "-bash", "fish"} {
		if !isShellCommand(sh) {
			t.Fatalf("expected %q to be a shell", sh)
		}
	}

	if isShellCommand("nvim") {
		t.Fatal("nvim should not be a shell")
	}

	if got := executableName("/usr/bin/htop -d 5"); got != "htop" {
		t.Fatalf("executableName: got %q", got)
	}
}

func TestNormalizedCommand(t *testing.T) {
	cases := []struct {
		restore, current, want string
	}{
		{"nvim", "zsh", "nvim"}, // restore wins when it is a real command
		{"zsh", "nvim", "nvim"}, // restore is a shell -> use current
		{"zsh", "bash", ""},     // both shells -> nothing to restore
		{`"htop"`, "", "htop"},  // quotes stripped
	}

	for _, c := range cases {
		if got := normalizedCommand(c.restore, c.current); got != c.want {
			t.Fatalf("normalizedCommand(%q,%q)=%q want %q", c.restore, c.current, got, c.want)
		}
	}
}

func TestParsePSLine(t *testing.T) {
	pid, ppid, stat, cmd, ok := parsePSLine("200 100 S+ nvim main.go")
	if !ok || pid != 200 || ppid != 100 || stat != "S+" || cmd != "nvim main.go" {
		t.Fatalf("parsePSLine: %d %d %q %q ok=%v", pid, ppid, stat, cmd, ok)
	}

	if _, _, _, _, ok := parsePSLine("bad line"); ok {
		t.Fatal("expected parse failure for short line")
	}
}

func TestPickForegroundCommand(t *testing.T) {
	lines := []string{
		"100 1 Ss zsh",    // the shell itself (panePID)
		"200 100 S+ nvim", // foreground child
	}

	if got := pickForegroundCommand(lines, 100); got != "nvim" {
		t.Fatalf("expected foreground nvim, got %q", got)
	}

	// Only a shell present -> nothing to restore.
	if got := pickForegroundCommand([]string{"100 1 Ss zsh"}, 100); got != "" {
		t.Fatalf("expected empty for shell-only, got %q", got)
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines("a\n\n  b  \nc\n")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitLines: %#v", got)
	}
}

func TestStripOptionPair(t *testing.T) {
	got := stripOptionPair([]string{"new-window", "-c", "/x", "-n", "name"}, "-c")
	want := []string{"new-window", "-n", "name"}

	if len(got) != len(want) {
		t.Fatalf("stripOptionPair length: %#v", got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stripOptionPair: %#v want %#v", got, want)
		}
	}
}

func TestNewSessionArgs(t *testing.T) {
	args := newSessionArgs("sess", snapshot.Window{
		Name:  "win",
		Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/work"}},
	})

	// Expect: new-session -d -s sess -n win -c /work
	joined := args
	if joined[0] != "new-session" || joined[2] != "-s" || joined[3] != "sess" ||
		joined[5] != "win" {
		t.Fatalf("unexpected new-session args: %#v", args)
	}

	hasPath := false
	for i := range args {
		if args[i] == "-c" && i+1 < len(args) && args[i+1] == "/work" {
			hasPath = true
		}
	}

	if !hasPath {
		t.Fatalf("expected -c /work in args: %#v", args)
	}
}

func TestInsideTmux(t *testing.T) {
	client := NewClient("tmux")

	t.Setenv("TMUX", "")

	if client.InsideTmux() {
		t.Fatal("InsideTmux must be false when $TMUX is empty")
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")

	if !client.InsideTmux() {
		t.Fatal("InsideTmux must be true when $TMUX is set")
	}
}

func TestAttachSessionExecsTmux(t *testing.T) {
	var (
		gotArgv0 string
		gotArgs  []string
	)

	origExec := attachExec
	attachExec = func(argv0 string, argv, _ []string) error {
		gotArgv0 = argv0
		gotArgs = argv

		return nil
	}

	origTTY := hasControllingTTY
	hasControllingTTY = func() bool { return true }

	t.Cleanup(func() {
		attachExec = origExec
		hasControllingTTY = origTTY
	})

	// "sh" resolves on every supported platform, so LookPath succeeds.
	if err := NewClient("sh").AttachSession("proj:2"); err != nil {
		t.Fatalf("attach session: %v", err)
	}

	if gotArgv0 == "" {
		t.Fatal("expected a resolved tmux binary path")
	}

	want := []string{gotArgv0, "attach-session", "-t", "=proj:2"}
	if len(gotArgs) != len(want) {
		t.Fatalf("argv = %v, want %v", gotArgs, want)
	}

	for i := range want {
		if gotArgs[i] != want[i] {
			t.Fatalf("argv = %v, want %v", gotArgs, want)
		}
	}
}

func TestAttachSessionWithoutTTYIsNoOp(t *testing.T) {
	called := false

	origExec := attachExec
	attachExec = func(string, []string, []string) error {
		called = true

		return nil
	}

	origTTY := hasControllingTTY
	hasControllingTTY = func() bool { return false }

	t.Cleanup(func() {
		attachExec = origExec
		hasControllingTTY = origTTY
	})

	// Without a controlling terminal, attach must be skipped (tmux would fail
	// with "open terminal failed"), leaving the session restored-but-detached.
	if err := NewClient("sh").AttachSession("proj"); err != nil {
		t.Fatalf("attach session: %v", err)
	}

	if called {
		t.Fatal("attach must not exec tmux without a controlling TTY")
	}
}
