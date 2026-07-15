package tmux

import (
	"errors"
	"fmt"
	"io"
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

	client.waitForRestoredCommands("sess", waitTestWindows())

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
	client.waitForRestoredCommands("sess", waitTestWindows())

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("wait ignored its timeout, took %v", elapsed)
	}

	if runner.calls == 0 {
		t.Fatal("expected at least one poll before giving up")
	}
}

func TestWaitForRestoredCommandsDisabled(t *testing.T) {
	t.Parallel()

	runner := &pollRunner{settleOn: 1, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreTimeout(0)

	client.waitForRestoredCommands("sess", waitTestWindows())

	if runner.calls != 0 {
		t.Fatalf("a disabled timeout must not poll, polled %d times", runner.calls)
	}
}

func TestWaitForRestoredCommandsSkipsHandlerActions(t *testing.T) {
	t.Parallel()

	runner := &pollRunner{settleOn: 1, before: "zsh", after: "cat"}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreHandler("echo")
	client.SetRestoreTimeout(2 * time.Second)

	client.waitForRestoredCommands("sess", waitTestWindows())

	if runner.calls != 0 {
		t.Fatalf("handler actions must not poll, polled %d times", runner.calls)
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

func TestExpectedPaneCommandsEmptyWithRestoreHandler(t *testing.T) {
	t.Parallel()

	client := NewClient("tmux")
	client.SetRestoreHandler("echo")

	if got := client.expectedPaneCommands(waitTestWindows()); len(got) != 0 {
		t.Fatalf("handler mode must have no settle expectations, got %#v", got)
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

func TestRestoreHandlerCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{name: "plain", source: "nvim", expected: "echo 'nvim'"},
		{name: "spaces", source: "nvim main.go", expected: "echo 'nvim main.go'"},
		{name: "single quote", source: "printf 'hello'", expected: "echo 'printf '\\''hello'\\'''"},
		{name: "semicolon", source: "one; two", expected: "echo 'one; two'"},
		{name: "dollar substitution", source: "$(touch file)", expected: "echo '$(touch file)'"},
		{name: "backticks", source: "`touch file`", expected: "echo '`touch file`'"},
		{name: "backslash", source: `printf \\n`, expected: `echo 'printf \\n'`},
		{name: "double quote", source: `printf "hello"`, expected: `echo 'printf "hello"'`},
		{name: "newline", source: "first\nsecond", expected: `echo 'first\x0asecond'`},
		{
			name:     "representative controls",
			source:   "nul\x00tab\tesc\x1bctrl-u\x15del\x7f",
			expected: `echo 'nul\x00tab\x09esc\x1bctrl-u\x15del\x7f'`,
		},
		{
			name:     "unicode",
			source:   "nvim café/\u65e5\u672c\u8a9e",
			expected: "echo 'nvim café/\u65e5\u672c\u8a9e'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := restoreHandlerCommand("  echo  ", test.source); got != test.expected {
				t.Fatalf("restoreHandlerCommand() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestEncodeHandlerArgumentEncodesEveryUnsafeByte(t *testing.T) {
	t.Parallel()

	unsafe := make([]byte, 0, 33)
	for b := range byte(' ') {
		unsafe = append(unsafe, b)
	}
	unsafe = append(unsafe, '\x7f')

	for _, b := range unsafe {
		encoded := encodeHandlerArgument(string([]byte{b}))
		want := fmt.Sprintf(`\x%02x`, b)
		if encoded != want {
			t.Fatalf("encodeHandlerArgument(%#02x) = %q, want %q", b, encoded, want)
		}
	}
}

func TestEncodeHandlerArgumentHasNoUnsafeBytes(t *testing.T) {
	t.Parallel()

	input := "safe\x00\n\t\x1b\x15\x7f café/\u65e5\u672c\u8a9e"
	encoded := encodeHandlerArgument(input)
	want := "safe\\x00\\x0a\\x09\\x1b\\x15\\x7f café/\u65e5\u672c\u8a9e"
	if encoded != want {
		t.Fatalf("encodeHandlerArgument() = %q, want %q", encoded, want)
	}

	for i := range len(encoded) {
		if encoded[i] < ' ' || encoded[i] == '\x7f' {
			t.Fatalf("encoded output contains unsafe byte %#02x at offset %d", encoded[i], i)
		}
	}
}

type restoreCommandRunner struct {
	calls  [][]string
	failAt int
	err    error
}

func (r *restoreCommandRunner) runCommand(args ...string) commandResult {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(r.calls) == r.failAt {
		return commandResult{err: r.err}
	}

	return commandResult{}
}

func restoreTestWindow(panes ...snapshot.Pane) snapshot.Window {
	return snapshot.Window{Index: 1, Panes: panes}
}

func commandCallsEqual(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			return false
		}
	}

	return true
}

func TestRestoreHandlerSelectsNormalizedSavedCommand(t *testing.T) {
	t.Parallel()

	runner := &restoreCommandRunner{}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreHandler("  cowsay -f tux  ")

	window := restoreTestWindow(
		snapshot.Pane{Index: 0, RestoreCmd: "nvim main.go", CurrentCmd: "nvim"},
		snapshot.Pane{Index: 1, RestoreCmd: "zsh", CurrentCmd: "htop"},
		snapshot.Pane{Index: 2},
		snapshot.Pane{Index: 3, RestoreCmd: "   ", CurrentCmd: "bash"},
		snapshot.Pane{Index: 4, RestoreCmd: "/bin/fish", CurrentCmd: "-zsh"},
	)

	if err := client.restoreWindowCommands("work", window, 1); err != nil {
		t.Fatalf("restoreWindowCommands: %v", err)
	}

	want := [][]string{
		{"tmux", "send-keys", "-l", "-t", "=work:1.0", "cowsay -f tux 'nvim main.go'"},
		{"tmux", "send-keys", "-t", "=work:1.0", "C-m"},
		{"tmux", "send-keys", "-l", "-t", "=work:1.1", "cowsay -f tux 'htop'"},
		{"tmux", "send-keys", "-t", "=work:1.1", "C-m"},
	}
	if !commandCallsEqual(runner.calls, want) {
		t.Fatalf("handler dispatch calls:\n got %q\nwant %q", runner.calls, want)
	}
}

func TestRestoreHandlerDispatchContainsNoSavedControlBytes(t *testing.T) {
	t.Parallel()

	unsafe := make([]byte, 0, 33)
	for b := range byte(' ') {
		unsafe = append(unsafe, b)
	}
	unsafe = append(unsafe, '\x7f')

	runner := &restoreCommandRunner{}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreHandler("echo")
	window := restoreTestWindow(snapshot.Pane{RestoreCmd: "cmd" + string(unsafe) + "end"})

	if err := client.restoreWindowCommands("work", window, 1); err != nil {
		t.Fatalf("restoreWindowCommands: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("got %d dispatch calls, want 2: %q", len(runner.calls), runner.calls)
	}

	line := runner.calls[0][len(runner.calls[0])-1]
	for i := range len(line) {
		if line[i] < ' ' || line[i] == '\x7f' {
			t.Fatalf("handler send-keys contains unsafe byte %#02x at offset %d", line[i], i)
		}
	}
}

func TestRestoreHandlerFiltersSavedCommandBeforeWrapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		allowlist []string
		denylist  []string
		wantCalls int
	}{
		{name: "saved command allowed", allowlist: []string{"nvim( .*)?"}, wantCalls: 2},
		{name: "handler prefix does not allow source", allowlist: []string{"echo.*"}},
		{
			name:      "deny wins",
			allowlist: []string{"nvim( .*)?"},
			denylist:  []string{"nvim .*"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &restoreCommandRunner{}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreHandler("echo")
			client.SetRestoreAllowlist(test.allowlist)
			client.SetRestoreDenylist(test.denylist)

			window := restoreTestWindow(snapshot.Pane{RestoreCmd: "nvim main.go"})
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}

			if len(runner.calls) != test.wantCalls {
				t.Fatalf(
					"got %d dispatch calls, want %d: %q",
					len(runner.calls),
					test.wantCalls,
					runner.calls,
				)
			}
		})
	}
}

func TestRestoreHandlerDisabledPreservesDirectDispatch(t *testing.T) {
	t.Parallel()

	for _, handler := range []string{"", "   \t\n"} {
		runner := &restoreCommandRunner{}
		client := NewClientWithRunner("tmux", runner)
		client.SetRestoreHandler(handler)

		window := restoreTestWindow(snapshot.Pane{RestoreCmd: "nvim main.go"})
		if err := client.restoreWindowCommands("work", window, 1); err != nil {
			t.Fatalf("restoreWindowCommands: %v", err)
		}

		want := [][]string{{"tmux", "send-keys", "-t", "=work:1.0", "nvim main.go", "C-m"}}
		if !commandCallsEqual(runner.calls, want) {
			t.Fatalf("direct dispatch calls:\n got %q\nwant %q", runner.calls, want)
		}
	}
}

func TestRestoreHandlerDispatchErrors(t *testing.T) {
	t.Parallel()

	dispatchErr := io.ErrUnexpectedEOF
	tests := []struct {
		name      string
		failAt    int
		wantCalls int
	}{
		{name: "literal text", failAt: 1, wantCalls: 1},
		{name: "enter", failAt: 2, wantCalls: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &restoreCommandRunner{failAt: test.failAt, err: dispatchErr}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreHandler("echo")

			window := restoreTestWindow(snapshot.Pane{RestoreCmd: "nvim main.go"})
			err := client.restoreWindowCommands("work", window, 1)
			if !errors.Is(err, dispatchErr) {
				t.Fatalf("restoreWindowCommands error = %v, want %v", err, dispatchErr)
			}

			if len(runner.calls) != test.wantCalls {
				t.Fatalf(
					"got %d dispatch calls, want %d: %q",
					len(runner.calls),
					test.wantCalls,
					runner.calls,
				)
			}
		})
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
		"100 1 Ss zsh",
		"300 100 S+ fish",
	}
	if got := pickForegroundCommand(launched, 100); got != "fish" {
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

type argsRunner struct{ calls [][]string }

func (r *argsRunner) runCommand(args ...string) commandResult {
	r.calls = append(r.calls, args)

	return commandResult{}
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
