package tmux

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type recordingRunner struct {
	calls         [][]string
	listPaneCalls int
}

func (r *recordingRunner) runCommand(args ...string) commandResult {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(args) > 1 && args[1] == "list-panes" {
		r.listPaneCalls++

		return commandResult{stdout: "1 0 claude\n"}
	}

	return commandResult{}
}

type fakeResolver struct {
	matchCmd string
	override string
}

func (f fakeResolver) Resolve(pane snapshot.Pane) string {
	if pane.CurrentCmd == f.matchCmd {
		return f.override
	}

	return ""
}

type countingResolver struct {
	matchCmd string
	override string
	calls    int
}

func (r *countingResolver) Resolve(pane snapshot.Pane) string {
	r.calls++
	if r.matchCmd == "" || pane.CurrentCmd == r.matchCmd {
		return r.override
	}

	return ""
}

func sentKeys(calls [][]string) []string {
	var out []string

	for _, call := range calls {
		if len(call) > 1 && call[1] == "send-keys" {
			out = append(out, call[len(call)-2])
		}
	}

	return out
}

func TestRestoreResolverOverridesCommand(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreResolver(fakeResolver{matchCmd: "claude", override: "claude --resume sess-1"})

	window := snapshot.Window{
		Index: 1,
		Panes: []snapshot.Pane{
			{Index: 0, CurrentCmd: "claude", RestoreCmd: "claude"},
			{Index: 1, CurrentCmd: "vim", RestoreCmd: "vim main.go"},
		},
	}

	if err := client.restoreWindowCommands("work", window, 1); err != nil {
		t.Fatalf("restoreWindowCommands: %v", err)
	}

	sent := sentKeys(runner.calls)
	if len(sent) != 2 {
		t.Fatalf("expected 2 send-keys, got %v", sent)
	}

	if sent[0] != "claude --resume sess-1" {
		t.Fatalf("claude pane should use the resolver override, got %q", sent[0])
	}

	if sent[1] != "vim main.go" {
		t.Fatalf("non-matching pane keeps the default command, got %q", sent[1])
	}
}

func TestEffectiveRestoreCommandFallsBack(t *testing.T) {
	t.Parallel()

	client := NewClientWithRunner("tmux", &recordingRunner{})

	pane := snapshot.Pane{CurrentCmd: "vim", RestoreCmd: "vim main.go"}
	if got := client.effectiveRestoreCommand(pane); got != "vim main.go" {
		t.Fatalf("default path changed, got %q", got)
	}

	client.SetRestoreResolver(fakeResolver{matchCmd: "claude", override: "x"})
	if got := client.effectiveRestoreCommand(pane); got != "vim main.go" {
		t.Fatalf("empty override should fall back, got %q", got)
	}
}

func TestRestoreResolverRespectsAllowlist(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreResolver(fakeResolver{matchCmd: "claude", override: "claude --resume s"})
	client.SetRestoreAllowlist([]string{"vim"})

	window := snapshot.Window{
		Index: 1,
		Panes: []snapshot.Pane{{Index: 0, CurrentCmd: "claude", RestoreCmd: "claude"}},
	}

	if err := client.restoreWindowCommands("work", window, 1); err != nil {
		t.Fatalf("restoreWindowCommands: %v", err)
	}

	for _, cmd := range sentKeys(runner.calls) {
		if strings.Contains(cmd, "claude") {
			t.Fatalf("allowlist must block the resolved claude command, sent %q", cmd)
		}
	}
}

func TestRestoreHandlerSourceSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		useResolver bool
		wantLine    string
		wantCalls   int
	}{
		{name: "saved zero value", wantLine: "echo 'claude'"},
		{
			name:        "resolved",
			useResolver: true,
			wantLine:    "echo 'claude --resume s'",
			wantCalls:   1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{}
			resolver := &countingResolver{matchCmd: "claude", override: "claude --resume s"}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreResolver(resolver)
			client.SetRestoreHandler("echo")
			client.SetRestoreHandlerUseResolver(test.useResolver)

			window := restoreTestWindow(
				snapshot.Pane{Index: 0, CurrentCmd: "claude", RestoreCmd: "claude"},
			)
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}

			want := [][]string{
				{"tmux", "send-keys", "-l", "-t", "=work:1.0", test.wantLine},
				{"tmux", "send-keys", "-t", "=work:1.0", "C-m"},
			}
			if !commandCallsEqual(runner.calls, want) {
				t.Fatalf("handler calls:\n got %q\nwant %q", runner.calls, want)
			}
			if resolver.calls != test.wantCalls {
				t.Fatalf("resolver calls: got %d want %d", resolver.calls, test.wantCalls)
			}
		})
	}
}

func TestRestoreHandlerResolvedSourceFallsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolver RestoreCommandResolver
	}{
		{name: "no resolver"},
		{name: "non-match", resolver: &countingResolver{matchCmd: "other", override: "override"}},
		{name: "missing metadata", resolver: &countingResolver{override: ""}},
		{name: "empty result", resolver: &countingResolver{override: ""}},
		{name: "whitespace result", resolver: &countingResolver{override: "  \t  "}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreHandler("echo")
			client.SetRestoreHandlerUseResolver(true)
			client.SetRestoreResolver(test.resolver)

			window := restoreTestWindow(
				snapshot.Pane{Index: 0, CurrentCmd: "claude", RestoreCmd: "claude --saved"},
			)
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}

			want := [][]string{
				{"tmux", "send-keys", "-l", "-t", "=work:1.0", "echo 'claude --saved'"},
				{"tmux", "send-keys", "-t", "=work:1.0", "C-m"},
			}
			if !commandCallsEqual(runner.calls, want) {
				t.Fatalf("fallback calls:\n got %q\nwant %q", runner.calls, want)
			}
		})
	}
}

func TestRestoreHandlerResolvedShellSourceIsSkipped(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "/bin/zsh", "fish -l"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{}
			resolver := &countingResolver{override: shell}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreResolver(resolver)
			client.SetRestoreHandler("echo")
			client.SetRestoreHandlerUseResolver(true)

			window := restoreTestWindow(snapshot.Pane{RestoreCmd: "nvim", CurrentCmd: "nvim"})
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}
			if resolver.calls != 1 {
				t.Fatalf("resolver calls: got %d want 1", resolver.calls)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("shell resolver result must be skipped, got %q", runner.calls)
			}
		})
	}
}

func TestRestoreHandlerFiltersSelectedSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		useResolver bool
		allowlist   []string
		wantLine    string
	}{
		{name: "saved allows saved", allowlist: []string{"claude"}, wantLine: "echo 'claude'"},
		{name: "saved blocks resolved pattern", allowlist: []string{"claude --resume .*"}},
		{
			name:        "resolved blocks saved pattern",
			useResolver: true,
			allowlist:   []string{"claude"},
		},
		{
			name:        "resolved allows resolved",
			useResolver: true,
			allowlist:   []string{"claude --resume .*"},
			wantLine:    "echo 'claude --resume s'",
		},
		{name: "handler prefix never allows saved", allowlist: []string{"echo.*"}},
		{
			name:        "handler prefix never allows resolved",
			useResolver: true,
			allowlist:   []string{"echo.*"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreResolver(
				fakeResolver{matchCmd: "claude", override: "claude --resume s"},
			)
			client.SetRestoreHandler("echo")
			client.SetRestoreHandlerUseResolver(test.useResolver)
			client.SetRestoreAllowlist(test.allowlist)

			window := restoreTestWindow(
				snapshot.Pane{CurrentCmd: "claude", RestoreCmd: "claude"},
			)
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}

			if test.wantLine == "" {
				if len(runner.calls) != 0 {
					t.Fatalf("blocked source dispatched %q", runner.calls)
				}

				return
			}

			if len(runner.calls) != 2 || runner.calls[0][len(runner.calls[0])-1] != test.wantLine {
				t.Fatalf("handler calls = %q, want line %q", runner.calls, test.wantLine)
			}
		})
	}
}

func TestRestoreHandlerSourceIsInertWithoutHandler(t *testing.T) {
	t.Parallel()

	for _, useResolver := range []bool{false, true} {
		t.Run(fmt.Sprintf("handler resolver %t", useResolver), func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{}
			resolver := &countingResolver{matchCmd: "claude", override: "claude --resume s"}
			client := NewClientWithRunner("tmux", runner)
			client.SetRestoreResolver(resolver)
			client.SetRestoreHandlerUseResolver(useResolver)
			client.SetRestoreAllowlist([]string{"claude --resume .*"})

			window := restoreTestWindow(
				snapshot.Pane{CurrentCmd: "claude", RestoreCmd: "claude"},
			)
			if err := client.restoreWindowCommands("work", window, 1); err != nil {
				t.Fatalf("restoreWindowCommands: %v", err)
			}

			want := [][]string{
				{"tmux", "send-keys", "-t", "=work:1.0", "--", "claude --resume s", "C-m"},
			}
			if !commandCallsEqual(runner.calls, want) {
				t.Fatalf("direct calls:\n got %q\nwant %q", runner.calls, want)
			}
			if resolver.calls != 1 {
				t.Fatalf("resolver calls: got %d want 1", resolver.calls)
			}

			expected := client.expectedPaneCommands([]snapshot.Window{window})
			if expected["1.0"] != "claude" {
				t.Fatalf("expected commands = %#v", expected)
			}

			client.SetRestoreTimeout(time.Second)
			client.waitForRestoredCommands("work", []snapshot.Window{window})
			if runner.listPaneCalls != 1 {
				t.Fatalf("settle list-panes calls: got %d want 1", runner.listPaneCalls)
			}
			if resolver.calls != 3 {
				t.Fatalf(
					"resolver calls after restore/expected/settle: got %d want 3",
					resolver.calls,
				)
			}
		})
	}
}
