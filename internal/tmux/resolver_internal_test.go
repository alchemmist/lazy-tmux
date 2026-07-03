package tmux

import (
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// recordingRunner captures every tmux invocation and returns success.
type recordingRunner struct {
	calls [][]string
}

func (r *recordingRunner) runCommand(args ...string) commandResult {
	r.calls = append(r.calls, append([]string(nil), args...))

	return commandResult{}
}

// fakeResolver overrides the restore command for a chosen current command.
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

func sentKeys(calls [][]string) []string {
	var out []string

	for _, call := range calls {
		if len(call) > 1 && call[1] == "send-keys" {
			// send-keys -t <target> <cmd> C-m
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

	// No resolver: default normalized command.
	pane := snapshot.Pane{CurrentCmd: "vim", RestoreCmd: "vim main.go"}
	if got := client.effectiveRestoreCommand(pane); got != "vim main.go" {
		t.Fatalf("default path changed, got %q", got)
	}

	// Resolver returning "" must defer to the default.
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
	client.SetRestoreAllowlist([]string{"vim"}) // claude not allowed

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
