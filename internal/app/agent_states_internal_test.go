package app

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestPickerDistinguishesUnverifiedAndPendingAgents(t *testing.T) {
	t.Parallel()
	registry := integration.NewRegistry(claude.New(t.TempDir(), t.TempDir()))
	windows := []snapshot.Window{
		{Index: 0, Panes: []snapshot.Pane{{CurrentCmd: "claude"}}},
		{
			Index: 1,
			Panes: []snapshot.Pane{
				{CurrentCmd: "claude", Meta: map[string]string{"agent.restore_pending": "1"}},
			},
		},
		{Index: 2, Panes: []snapshot.Pane{{CurrentCmd: "zsh"}}},
	}
	got := windowStatuses(registry, windows)
	if got[0] != picker.StatusNeedsSetup || got[1] != picker.StatusRestorePending ||
		got[2] != picker.StatusNone {
		t.Fatalf("unexpected states: %v", got)
	}
}

func TestQuickPickerIncludesClaudeActivity(t *testing.T) {
	t.Parallel()
	app, _ := newTestApp(t)
	registry := integration.NewRegistry(claude.New(t.TempDir(), t.TempDir()))
	pane := snapshot.Pane{
		CurrentCmd: "claude",
		Agent: &snapshot.AgentSession{
			Version: 1,
			Kind:    "claude",
			ID:      "id",
			Home:    "/home/claude",
			CWD:     "/workspace",
			Argv:    []string{"claude", "--resume", "id"},
			Source:  "hook-v1",
			Status:  "working",
		},
	}
	if err := app.store.SaveSession(
		snapshot.SessionSnapshot{
			SessionName: "work",
			Windows:     []snapshot.Window{{Panes: []snapshot.Pane{pane}}},
		},
	); err != nil {
		t.Fatal(err)
	}
	if !app.sessionHasWorkingAgent(registry, "work", true) {
		t.Fatal("Claude activity excluded from quick picker")
	}
	if app.sessionHasWorkingAgent(registry, "work", false) {
		t.Fatal("offline snapshot counted as working")
	}
}
