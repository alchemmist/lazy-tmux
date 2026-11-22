package tmux

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

// No agent runs in these panes. Authentication must fail before either the
// existing pane binding or the saved session can acquire an agent identity.
//
//nolint:paralleltest // IsolatedTmux sets the process environment.
func TestHookRejectsUnownedPane(t *testing.T) {
	testutil.IsolatedTmux(t)
	client := NewClient("tmux")
	for _, kind := range []string{"claude", "codex"} {
		pane := strings.TrimSpace(
			testutil.Tmux(
				t,
				"new-session",
				"-d",
				"-P",
				"-F",
				"#{pane_id}",
				"-s",
				kind,
				"sleep",
				"60",
			),
		)
		testutil.Tmux(t, "set-option", "-p", "-t", pane, agentBindingOption, "previous")
		err := client.ApplyAgentHook(
			kind,
			t.TempDir(),
			pane,
			AgentHookEvent{Event: sessionStartEvent, SessionID: "unverified", CWD: t.TempDir()},
		)
		if !errors.Is(err, errHookOwner) {
			t.Fatalf("%s: got %v, want ownership refusal", kind, err)
		}
		if got := strings.TrimSpace(
			testutil.Tmux(t, "show-options", "-pv", "-t", pane, agentBindingOption),
		); got != "previous" {
			t.Fatalf("refused hook overwrote binding: %q", got)
		}
		state, err := client.CaptureSession(kind)
		if err != nil {
			t.Fatal(err)
		}
		saved := saveAndLoadHookSnapshot(t, state)
		if saved.Windows[0].Panes[0].Agent != nil {
			t.Fatalf("stored unverified identity: %+v", saved)
		}
	}
}

// Use launch and owner records as data, not an executable claiming to be the
// agent. Real-client ownership and resume are covered by TestRealAgentLifecycle.
//
//nolint:paralleltest // Subtests share the isolated tmux server and set the hook environment.
func TestOfflineHookRecordRoundTrip(t *testing.T) {
	testutil.IsolatedTmux(t)
	home, cwd := t.TempDir(), t.TempDir()
	registry := integration.NewRegistry(codex.New(home), claude.New(home, t.TempDir()))
	for _, kind := range []string{"claude", "codex"} {
		for _, id := range []string{"first", "second"} {
			t.Run(
				kind+"/"+id,
				func(t *testing.T) { checkHookRecord(t, registry, home, cwd, kind, id) },
			)
		}
	}
}

func checkHookRecord(t *testing.T, registry *integration.Registry, home, cwd, kind, id string) {
	t.Helper()
	name := kind + "-" + id
	pane := strings.TrimSpace(
		testutil.Tmux(t, "new-session", "-d", "-P", "-F", "#{pane_id}", "-s", name),
	)
	owner := LiveBinding{
		Version:    1,
		PaneID:     pane,
		SocketPath: NewClient("tmux").SocketPath(),
		PID:        20,
		StartedAt:  "start",
	}
	event := AgentHookEvent{Event: sessionStartEvent, SessionID: id, CWD: cwd}
	record, changed, err := reduceAgentHook(LiveBinding{}, owner, kind, home, event, 1)
	if err != nil || !changed {
		t.Fatalf("start: %v %v", changed, err)
	}
	override := ""
	if id == "second" {
		override = home
	}
	t.Setenv("CLAUDE_CONFIG_DIR", override)
	if err := setHookLaunchArgs(&record.Session, event, kind, []string{kind}); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(t, hookPublishArgs(pane, string(data), event.Event, "token")...)
	raw := testutil.Tmux(t, "show-options", "-pv", "-t", pane, agentBindingOption)
	if strings.TrimSpace(
		testutil.Tmux(t, "show-options", "-pv", "-t", pane, restoreAckOption),
	) != "token" {
		t.Fatal("restore not acknowledged")
	}
	var published LiveBinding
	if err := json.Unmarshal([]byte(raw), &published); err != nil {
		t.Fatal(err)
	}
	state := snapshot.SessionSnapshot{
		SessionName: name,
		Windows: []snapshot.Window{
			{Panes: []snapshot.Pane{{CurrentCmd: kind, Agent: &published.Session}}},
		},
	}
	saved := saveAndLoadHookSnapshot(t, state).Windows[0].Panes[0]
	if saved.Agent == nil || saved.Agent.ID != id || saved.Agent.CWD != cwd ||
		saved.Agent.Kind != kind {
		t.Fatalf("lost identity: %+v", saved)
	}
	command, err := registry.ResolveChecked(saved)
	if err != nil || !strings.Contains(command, "'"+kind+"'") ||
		!strings.Contains(command, "'"+id+"'") {
		t.Fatalf("wrong resume: %s %v", command, err)
	}
	if kind == "claude" {
		prefix := "env -u CLAUDE_CONFIG_DIR "
		if override != "" {
			prefix = "env 'CLAUDE_CONFIG_DIR=" + override + "' "
		}
		if saved.Agent.ClaudeConfigDir == nil || *saved.Agent.ClaudeConfigDir != override ||
			!strings.HasPrefix(command, prefix) {
			t.Fatalf("lost Claude config provenance: %s", command)
		}
	}
}

func saveAndLoadHookSnapshot(
	t *testing.T,
	state snapshot.SessionSnapshot,
) snapshot.SessionSnapshot {
	t.Helper()
	storage := store.New(t.TempDir())
	if err := storage.SaveSession(state); err != nil {
		t.Fatal(err)
	}
	saved, err := storage.LoadSession(state.SessionName)
	if err != nil {
		t.Fatal(err)
	}

	return saved
}
