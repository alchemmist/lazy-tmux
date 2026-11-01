package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
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
	live := []snapshot.Pane{pane}
	registry := integration.NewRegistry(
		claude.New("", "").WithStatusLoader(func() ([]snapshot.Pane, error) { return live, nil }),
	)
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
	live = nil
	if app.sessionHasWorkingAgent(registry, "work", true) {
		t.Fatal("exited owner still counted as working")
	}
}

func TestPiPickerStatusRefreshWithoutAutosave(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	file := filepath.Join(home, "history.jsonl")
	if err := os.WriteFile(
		file,
		[]byte(`{"type":"session","id":"pi-session","cwd":"`+home+`"}`+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	pane := snapshot.Pane{
		CurrentCmd:  "node",
		RestoreCmd:  "pi",
		CurrentPath: home,
		Meta:        map[string]string{"pi.session_file": file, "pi.session_id": "pi-session"},
	}
	live := snapshot.Pane{
		CurrentPath:  home,
		LiveStatusID: "pi-session",
		LiveStatus:   "working",
		Meta:         map[string]string{"pi.session_file": file},
	}
	registry := integration.NewRegistry(
		pi.New(home, home).
			WithStatusLoader(func() ([]snapshot.Pane, error) { return []snapshot.Pane{live}, nil }),
	)
	app, _ := newTestApp(t)
	windows := []snapshot.Window{{Index: 1, Panes: []snapshot.Pane{pane}}}
	if err := app.store.SaveSession(
		snapshot.SessionSnapshot{SessionName: "pi", Windows: windows},
	); err != nil {
		t.Fatal(err)
	}
	for state, want := range map[string]picker.WindowStatus{
		"working": picker.StatusWorking, "awaiting_input": picker.StatusAwaitingInput,
		"awaiting_decision": picker.StatusAwaitingDecision, "idle": picker.StatusIdle, "error": picker.StatusError,
	} {
		live.LiveStatus = state
		scope := registry.Scope()
		if got := windowStatuses(scope, windows)[1]; got != want {
			t.Fatalf("%s: got %v, want %v", state, got, want)
		}
		if got := app.sessionHasWorkingAgent(scope, "pi", true); got != (state == "working") {
			t.Fatalf("quick picker state %s = %v", state, got)
		}
		if app.sessionHasWorkingAgent(scope, "pi", false) {
			t.Fatal("offline Pi animated")
		}
	}
	live.LiveStatus = ""
	if got := windowStatuses(registry.Scope(), windows)[1]; got != picker.StatusNone {
		t.Fatalf("verified legacy Pi incorrectly needs setup: %v", got)
	}
}
