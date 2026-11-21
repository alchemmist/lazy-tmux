package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestListSessionsTreatsNoServerAsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	testutil.Tmux(t, "kill-server")
	got, err := NewClient("tmux").ListSessions()
	if err != nil || got != nil {
		t.Fatalf("list after shutdown: got %#v, %v", got, err)
	}
}

var (
	errLinuxNoServer    = errors.New("no server running on /tmp/tmux-1000/default")
	errMacMissingSocket = errors.New(
		"error connecting to /private/tmp/tmux-501/default (No such file or directory)",
	)
)

func TestNoServerErrorPlatformMessages(t *testing.T) {
	t.Parallel()
	for _, err := range []error{errLinuxNoServer, errMacMissingSocket} {
		if !isNoServerError(err) {
			t.Errorf("not recognized: %v", err)
		}
	}
}

func TestListSessionsPropagatesRealErrors(t *testing.T) {
	t.Parallel()
	client := NewClient(filepath.Join(t.TempDir(), "missing-tmux"))
	if _, err := client.ListSessions(); err == nil {
		t.Fatal("missing executable must propagate an error")
	}
}

func TestSessionsLastAttached(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	const name = "a|b"
	testutil.Tmux(t, "new-session", "-d", "-s", name)
	testutil.Tmux(t, "new-session", "-d", "-s", "never")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "-C", "attach-session", "-t", "="+name)
	cmd.Stdin = strings.NewReader("detach-client\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("control client: %v: %s", err, out)
	}
	stamp := strings.TrimSpace(
		testutil.Tmux(t, "display-message", "-p", "-t", "="+name+":", "#{session_last_attached}"),
	)
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("tmux")
	got := client.SessionsLastAttached()
	if len(got) != 1 || !got[name].Equal(time.Unix(seconds, 0)) {
		t.Fatalf("last attached=%v; want only %q at %d", got, name, seconds)
	}
	testutil.Tmux(t, "kill-server")
	if got := client.SessionsLastAttached(); got != nil {
		t.Fatalf("missing server: %v", got)
	}
}

func TestWaitForUnstartedCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "sess", "sleep 60")
	client := NewClient("tmux")
	windows := []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0, RestoreCmd: "cat"}}}}
	const timeout = 150 * time.Millisecond
	client.SetRestoreTimeout(timeout)
	start := time.Now()
	client.waitForRestoredCommands(t.Context(), "sess", windows)
	if elapsed := time.Since(start); elapsed < timeout || elapsed > time.Second {
		t.Fatalf("timeout wait: %v", elapsed)
	}
	client.SetRestoreTimeout(10 * time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start = time.Now()
	client.waitForRestoredCommands(ctx, "sess", windows)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled wait: %v", elapsed)
	}
	client.SetRestoreTimeout(0)
	start = time.Now()
	client.waitForRestoredCommands(t.Context(), "sess", windows)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("disabled wait: %v", elapsed)
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
		"200 100 S antex",
	}

	if got := pickForegroundCommand(lines, 100); got != "antex" {
		t.Fatalf("expected child antex, got %q", got)
	}
}

func TestProcessSnapshotResolvesIndependentPaneTrees(t *testing.T) {
	t.Parallel()

	snapshot := newProcessSnapshot([]string{
		"100 1 Ss zsh",
		"101 100 S+ nvim main.go",
		"200 1 Ss zsh",
		"201 200 S+ antex",
		"300 1 S unrelated",
	})

	if got := snapshot.foregroundCommand(100); got != "nvim main.go" {
		t.Fatalf("pane 100 foreground = %q", got)
	}
	if got := snapshot.foregroundCommand(200); got != "antex" {
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
		"200 100 S+ antex",
		"300 1 S+ unrelated",
	}

	tree := processTreeLines(lines, 100)
	if got := pickForegroundCommand(tree, 100); got != "antex" {
		t.Fatalf("expected antex from pane tree, got %q", got)
	}
	if len(tree) != 1 {
		t.Fatalf("expected one descendant, got %v", tree)
	}
}

func TestCapturePaneAndScrollback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(
		testutil.Tmux(
			t,
			"new-session",
			"-d",
			"-s",
			"capture",
			"-P",
			"-F",
			"#{pane_id}",
			"-c",
			dir,
			"-x",
			"40",
			"-y",
			"5",
			`printf '\033[31mhistory-marker\033[0m\n'; seq 1 40; exec cat`,
		),
	)
	client := NewClient("tmux")
	for deadline := time.Now().Add(5 * time.Second); ; {
		got, err := client.CapturePane(pane)
		if err != nil {
			t.Fatal(err)
		}
		screen := testutil.Tmux(t, "capture-pane", "-p", "-t", pane)
		if got.CurrentCmd == "cat" && strings.Contains(screen, "40") {
			if got.CurrentPath != dir || got.ProcessPID <= 0 || !got.IsActive || got.Meta != nil {
				t.Fatalf("captured pane: %+v", got)
			}

			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane never became ready: %+v, screen %q", got, screen)
		}
		time.Sleep(20 * time.Millisecond)
	}
	all, err := client.CapturePaneScrollback(pane, 0)
	if err != nil {
		t.Fatal(err)
	}
	recent, err := client.CapturePaneScrollback(pane, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(all, "\x1b[31mhistory-marker") ||
		strings.Contains(recent, "history-marker") ||
		!strings.Contains(recent, "40") {
		t.Fatalf("default history %q; recent %q", all, recent)
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
