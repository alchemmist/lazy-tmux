package tmux

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errFakeRunner = errors.New("fake runner failure")

type errRunner struct{ msg string }

func (r errRunner) runCommand(_ ...string) commandResult {
	return commandResult{err: fmt.Errorf("%s: %w", r.msg, errFakeRunner)}
}

func TestListSessionsTreatsNoServerAsEmpty(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"linux no server running": "tmux list-sessions: exit status 1 (no server running on /tmp/tmux-1000/default)",
		"macos socket missing": "tmux list-sessions -F #{session_name}: exit status 1 " +
			"(error connecting to /private/tmp/tmux-501/default (No such file or directory))",
	}

	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := NewClientWithRunner("tmux", errRunner{msg: msg})

			got, err := client.ListSessions()
			if err != nil {
				t.Fatalf("no-server error must be swallowed, got %v", err)
			}

			if got != nil {
				t.Fatalf("expected nil sessions, got %#v", got)
			}
		})
	}
}

func TestListSessionsPropagatesRealErrors(t *testing.T) {
	t.Parallel()

	client := NewClientWithRunner("tmux", errRunner{msg: "tmux: command not found"})

	if _, err := client.ListSessions(); err == nil {
		t.Fatal("a real error must propagate, not be swallowed as no-server")
	}
}

type stdoutRunner struct {
	out string
	err error
}

func (r stdoutRunner) runCommand(_ ...string) commandResult {
	return commandResult{stdout: r.out, err: r.err}
}

var errFakeNoServer = errors.New("no server running")

func TestSessionsLastAttached(t *testing.T) {
	t.Parallel()

	out := "1751385600" + fieldSep + "work\n" +
		"0" + fieldSep + "never\n" +
		"1751472000" + fieldSep + "a" + fieldSep + "b\n"

	got := NewClientWithRunner("tmux", stdoutRunner{out: out}).SessionsLastAttached()

	if len(got) != 2 {
		t.Fatalf("expected 2 attached sessions (never-attached dropped), got %#v", got)
	}

	if want := time.Unix(1751385600, 0).UTC(); !got["work"].Equal(want) {
		t.Fatalf("work: got %v want %v", got["work"], want)
	}

	if _, ok := got["a"+fieldSep+"b"]; !ok {
		t.Fatalf("expected name with separator preserved, got %#v", got)
	}

	if _, ok := got["never"]; ok {
		t.Fatal("a never-attached session (epoch 0) must be omitted")
	}
}

func TestSessionsLastAttachedBestEffortOnError(t *testing.T) {
	t.Parallel()

	got := NewClientWithRunner(
		"tmux",
		stdoutRunner{err: errFakeNoServer},
	).
		SessionsLastAttached()

	if got != nil {
		t.Fatalf("expected nil on error, got %#v", got)
	}
}

type pollRunner struct {
	calls    int
	settleOn int
	before   string
	after    string
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
	t.Parallel()

	runner := &pollRunner{settleOn: 3, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(2 * time.Second)

	client.waitForRestoredCommands(context.Background(), "sess", waitTestWindows())

	if runner.calls < 3 {
		t.Fatalf("expected to poll until command started, polled %d times", runner.calls)
	}
}

func TestWaitForRestoredCommandsRespectsTimeout(t *testing.T) {
	t.Parallel()

	runner := &pollRunner{settleOn: 1 << 30, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(150 * time.Millisecond)

	start := time.Now()
	client.waitForRestoredCommands(context.Background(), "sess", waitTestWindows())

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("wait ignored its timeout, took %v", elapsed)
	}

	if runner.calls == 0 {
		t.Fatal("expected at least one poll before giving up")
	}
}

func TestWaitForRestoredCommandsCancels(t *testing.T) {
	t.Parallel()

	runner := &pollRunner{settleOn: 1 << 30, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(10 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	client.waitForRestoredCommands(ctx, "sess", waitTestWindows())

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled wait must return promptly despite 10s timeout, took %v", elapsed)
	}
}

func TestWaitForRestoredCommandsDisabled(t *testing.T) {
	t.Parallel()

	runner := &pollRunner{settleOn: 1, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(0)

	client.waitForRestoredCommands(context.Background(), "sess", waitTestWindows())

	if runner.calls != 0 {
		t.Fatalf("a disabled timeout must not poll, polled %d times", runner.calls)
	}
}

func TestFieldSepIsPrintableASCII(t *testing.T) {
	t.Parallel()

	if len(fieldSep) == 0 {
		t.Fatal("fieldSep must not be empty")
	}

	for _, r := range fieldSep {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("fieldSep must be printable ASCII, got %q", fieldSep)
		}
	}
}

func TestSplitFieldsNKeepsTrailingFieldIntact(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	windows := []snapshot.Window{
		{Index: 1, IsActive: false, ActivePane: 0},
		{Index: 2, IsActive: true, ActivePane: 3},
	}

	win, pane := currentWindowPane(windows)
	if win != 2 || pane != 3 {
		t.Fatalf("expected active window 2 pane 3, got %d/%d", win, pane)
	}

	win, pane = currentWindowPane([]snapshot.Window{{Index: 5, ActivePane: 1}})
	if win != 5 || pane != 1 {
		t.Fatalf("expected fallback to first window 5/1, got %d/%d", win, pane)
	}

	win, pane = currentWindowPane(nil)
	if win != 0 || pane != 0 {
		t.Fatalf("expected 0/0 for no windows, got %d/%d", win, pane)
	}
}

func TestExpectedPaneCommands(t *testing.T) {
	t.Parallel()

	windows := []snapshot.Window{
		{
			Index: 1,
			Panes: []snapshot.Pane{
				{Index: 0, RestoreCmd: "nvim main.go"},
				{Index: 1, RestoreCmd: "fish"},
			},
		},
		{
			Index: 2,
			Panes: []snapshot.Pane{
				{Index: 0, CurrentCmd: "htop -d 5"},
				{Index: 1, CurrentCmd: "zsh"},
			},
		},
	}

	got := NewClient("tmux").expectedPaneCommands(windows)

	want := map[string]string{"1.0": "nvim", "1.1": "fish", "2.0": "htop"}
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
	t.Parallel()

	client := NewClient("tmux")

	if !client.commandAllowed("nvim main.go") || !client.commandAllowed("rm -rf x") {
		t.Fatal("with no allowlist, all commands must be allowed")
	}

	client.SetRestoreAllowlist([]string{"nvim( .*)?", "ssh .*", "htop"})

	for _, allowed := range []string{"nvim", "nvim main.go", "ssh host", "htop"} {
		if !client.commandAllowed(allowed) {
			t.Fatalf("%q should be allowed", allowed)
		}
	}

	for _, blocked := range []string{"nvimble", "htop -d 5", "ssh", "node app.js"} {
		if client.commandAllowed(blocked) {
			t.Fatalf("%q should be blocked", blocked)
		}
	}

	client.SetRestoreAllowlist([]string{})

	if client.commandAllowed("nvim") {
		t.Fatal("an empty allowlist must block all commands")
	}

	client.SetRestoreAllowlist(nil)

	if !client.commandAllowed("nvim") {
		t.Fatal("a nil allowlist must allow all commands again")
	}
}

func TestRestoreDenylist(t *testing.T) {
	t.Parallel()

	client := NewClient("tmux")

	if !client.commandAllowed("node app.js") {
		t.Fatal("with no denylist, all commands must be allowed")
	}

	client.SetRestoreDenylist(
		[]string{`node ./scripts/heavy-watch\.js --poll`, "htop( .*)?", "npm .*"},
	)

	for _, blocked := range []string{
		"node ./scripts/heavy-watch.js --poll", "htop", "htop -d 5", "npm install",
	} {
		if client.commandAllowed(blocked) {
			t.Fatalf("%q should be blocked", blocked)
		}
	}

	for _, allowed := range []string{
		"node ./scripts/heavy-watch.js", "node app.js", "nvim", "npm",
	} {
		if !client.commandAllowed(allowed) {
			t.Fatalf("%q should be allowed", allowed)
		}
	}

	client.SetRestoreDenylist(nil)

	if !client.commandAllowed("node app.js") {
		t.Fatal("a nil denylist must allow all commands again")
	}
}

func TestRestoreDenylistWinsOverAllowlist(t *testing.T) {
	t.Parallel()

	client := NewClient("tmux")

	client.SetRestoreAllowlist([]string{"nvim( .*)?", "node .*"})
	client.SetRestoreDenylist([]string{"node .*"})

	if !client.commandAllowed("nvim main.go") {
		t.Fatal("nvim is allowed and not denied -> should be restored")
	}

	if client.commandAllowed("node app.js") {
		t.Fatal("node is denied -> must be blocked even though the allowlist permits it")
	}

	if client.commandAllowed("htop") {
		t.Fatal("htop is not in the allowlist -> should stay blocked")
	}
}

func TestExpectedPaneCommandsRespectsAllowlist(t *testing.T) {
	t.Parallel()

	windows := []snapshot.Window{
		{
			Index: 1,
			Panes: []snapshot.Pane{
				{Index: 0, RestoreCmd: "nvim main.go"},
				{Index: 1, RestoreCmd: "htop"},
			},
		},
	}

	client := NewClient("tmux")
	client.SetRestoreAllowlist([]string{"nvim .*"})

	got := client.expectedPaneCommands(windows)

	if len(got) != 1 || got["1.0"] != "nvim" {
		t.Fatalf("allowlist should keep only nvim, got %#v", got)
	}
}

func TestCompileCommandPatternsSkipsInvalid(t *testing.T) {
	t.Parallel()

	client := NewClient("tmux")
	client.SetRestoreDenylist([]string{"node .*", "(", "  "})

	if client.commandAllowed("node app.js") {
		t.Fatal("valid pattern in the list must still block")
	}

	if !client.commandAllowed("nvim") {
		t.Fatal("an invalid/empty pattern must be skipped, not match everything")
	}
}

func TestSessionTargets(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	cases := []struct {
		restore, current, want string
	}{
		{"nvim", "zsh", "nvim"},
		{"zsh", "nvim", "nvim"},
		{"fish", "zsh", "fish"},
		{"", "zsh", ""},
		{`"htop"`, "", "htop"},
	}

	for _, c := range cases {
		if got := normalizedCommand(c.restore, c.current); got != c.want {
			t.Fatalf("normalizedCommand(%q,%q)=%q want %q", c.restore, c.current, got, c.want)
		}
	}
}

func TestParsePSLine(t *testing.T) {
	t.Parallel()

	pid, ppid, stat, cmd, ok := parsePSLine("200 100 S+ nvim main.go")
	if !ok || pid != 200 || ppid != 100 || stat != "S+" || cmd != "nvim main.go" {
		t.Fatalf("parsePSLine: %d %d %q %q ok=%v", pid, ppid, stat, cmd, ok)
	}

	if _, _, _, _, ok := parsePSLine("bad line"); ok {
		t.Fatal("expected parse failure for short line")
	}
}

func TestPickForegroundCommand(t *testing.T) {
	t.Parallel()

	lines := []string{
		"100 1 Ss zsh",
		"200 100 S+ nvim",
	}

	if got := pickForegroundCommand(lines, 100); got != "nvim" {
		t.Fatalf("expected foreground nvim, got %q", got)
	}

	if got := pickForegroundCommand([]string{"100 1 Ss zsh"}, 100); got != "" {
		t.Fatalf("expected empty for shell-only, got %q", got)
	}

	launched := []string{
		"101 1 Ss zsh",
		"300 101 S+ fish",
	}
	if got := pickForegroundCommand(launched, 101); got != "fish" {
		t.Fatalf("expected launched fish, got %q", got)
	}

	helper := []string{
		"100 1 Ss zsh",
		"301 100 S zsh -c helper",
	}
	if got := pickForegroundCommand(helper, 100); got != "" {
		t.Fatalf("expected empty for background shell helper, got %q", got)
	}

	nested := []string{
		"100 1 Ss zsh",
		"300 100 S fish",
		"400 300 S+ nvim",
	}
	if got := pickForegroundCommand(nested, 100); got != "nvim" {
		t.Fatalf("expected nvim over launched shell, got %q", got)
	}
}

func TestPickForegroundCommandFromProcessTree(t *testing.T) {
	t.Parallel()

	lines := []string{
		"100 1 Ss zsh",
		"200 100 S codex",
	}

	if got := pickForegroundCommand(lines, 100); got != "codex" {
		t.Fatalf("expected child codex, got %q", got)
	}
}

func TestProcessSnapshotResolvesIndependentPaneTrees(t *testing.T) {
	t.Parallel()

	snapshot := newProcessSnapshot([]string{
		"100 1 Ss zsh",
		"101 100 S+ nvim main.go",
		"200 1 Ss zsh",
		"201 200 S+ codex",
		"300 1 S unrelated",
	})

	if got := snapshot.foregroundCommand(100); got != "nvim main.go" {
		t.Fatalf("pane 100 foreground = %q", got)
	}
	if got := snapshot.foregroundCommand(200); got != "codex" {
		t.Fatalf("pane 200 foreground = %q", got)
	}
}

func BenchmarkProcessSnapshotForegroundCommands(b *testing.B) {
	lines := make([]string, 0, 2000)
	for index := 1; index <= 1000; index++ {
		root := index * 2
		lines = append(
			lines,
			fmt.Sprintf("%d 1 Ss zsh", root),
			fmt.Sprintf("%d %d S+ command-%d", root+1, root, index),
		)
	}

	b.ReportAllocs()
	for b.Loop() {
		snapshot := newProcessSnapshot(lines)
		for root := 2; root <= 200; root += 2 {
			_ = snapshot.foregroundCommand(root)
		}
	}
}

func TestProcessTreeLinesExcludesUnrelatedProcesses(t *testing.T) {
	t.Parallel()

	lines := []string{
		"100 1 Ss zsh",
		"200 100 S+ codex",
		"300 1 S+ unrelated",
	}

	tree := processTreeLines(lines, 100)
	if got := pickForegroundCommand(tree, 100); got != "codex" {
		t.Fatalf("expected codex from pane tree, got %q", got)
	}
	if len(tree) != 1 {
		t.Fatalf("expected one descendant, got %v", tree)
	}
}

type argsRunner struct {
	calls [][]string
	out   string
}

func (r *argsRunner) runCommand(args ...string) commandResult {
	r.calls = append(r.calls, args)

	return commandResult{stdout: r.out}
}

func TestCapturePane(t *testing.T) {
	t.Parallel()

	runner := &argsRunner{out: "codex|/workspace|thread-id\n"}
	client := NewClientWithRunner("tmux", runner)

	got, err := client.CapturePane("%7")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	want := snapshot.Pane{
		Index:       0,
		CurrentPath: "/workspace",
		CurrentCmd:  "codex",
		RestoreCmd:  "",
		Scrollback:  nil,
		IsActive:    true,
		Meta: map[string]string{
			snapshot.CodexSessionIDMetaKey: "thread-id",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CapturePane() = %#v; want %#v", got, want)
	}

	wantArgs := []string{
		"tmux",
		"display-message",
		"-p",
		"-t",
		"%7",
		"#{pane_current_command}|#{pane_current_path}|#{@codex_thread_id}",
	}
	if len(runner.calls) != 1 || !slices.Equal(runner.calls[0], wantArgs) {
		t.Fatalf("CapturePane args = %q; want %q", runner.calls, wantArgs)
	}
}

func TestCapturePaneScrollbackArgs(t *testing.T) {
	t.Parallel()

	runner := &argsRunner{}
	client := NewClientWithRunner("tmux", runner)

	if _, err := client.CapturePaneScrollback("sess:1.0", 0); err != nil {
		t.Fatalf("CapturePaneScrollback: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 tmux call, got %d", len(runner.calls))
	}

	want := []string{"tmux", "capture-pane", "-p", "-e", "-J", "-S", "-5000", "-t", "sess:1.0"}
	if got := runner.calls[0]; !slices.Equal(got, want) {
		t.Fatalf("capture-pane args:\n got %q\nwant %q", got, want)
	}
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	got := splitLines("a\n\n  b  \nc\n")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitLines: %#v", got)
	}
}

func TestStripOptionPair(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	args := newSessionArgs("sess", snapshot.Window{
		Name:  "win",
		Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/work"}},
	})

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

func TestSynchronizeWindowSize(t *testing.T) {
	t.Parallel()

	runner := &argsRunner{}
	client := NewClientWithRunner("tmux", runner)

	if err := client.SynchronizeWindowSize("proj:2"); err != nil {
		t.Fatalf("SynchronizeWindowSize: %v", err)
	}

	want := [][]string{
		{"tmux", "resize-window", "-A", "-t", "=proj:2"},
		{"tmux", "set-option", "-wu", "-t", "=proj:2", "window-size"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("SynchronizeWindowSize calls = %q; want %q", runner.calls, want)
	}
}

//nolint:paralleltest // stubs the package-level attachExec/hasControllingTTY seams
func TestAttachSessionExecsTmux(
	t *testing.T,
) {
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

//nolint:paralleltest // stubs the package-level attachExec/hasControllingTTY seams
func TestAttachSessionWithoutTTYIsNoOp(
	t *testing.T,
) {
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

	if err := NewClient("sh").AttachSession("proj"); err != nil {
		t.Fatalf("attach session: %v", err)
	}

	if called {
		t.Fatal("attach must not exec tmux without a controlling TTY")
	}
}
