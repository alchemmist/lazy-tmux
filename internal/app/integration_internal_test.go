package app

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func claudePane() snapshot.Pane {
	return snapshot.Pane{
		CurrentCmd: "claude",
		Agent: &snapshot.AgentSession{
			Version: 1,
			Kind:    "claude",
			ID:      "sess-9",
			Home:    "/home/claude",
			CWD:     "/workspace",
			Argv:    []string{"claude", "--resume", "sess-9"},
			Source:  "hook-v1",
		},
	}
}

func TestBuildRegistryEnabledResolvesClaude(
	t *testing.T,
) {
	t.Parallel()

	reg := buildRegistry(config.IntegrationsConfig{
		Enabled: true,
		Claude:  config.ClaudeIntegrationConfig{Enabled: true, Home: "~/.claude"},
	}, "", nil, nil)

	if got := reg.Resolve(
		claudePane(),
	); got != "env 'CLAUDE_CONFIG_DIR=/home/claude' 'claude' '--resume' 'sess-9'" {
		t.Fatalf("enabled claude should resolve resume command, got %q", got)
	}
}

func TestBuildRegistryMasterSwitchOff(
	t *testing.T,
) {
	t.Parallel()

	reg := buildRegistry(config.IntegrationsConfig{
		Enabled: false,
		Claude:  config.ClaudeIntegrationConfig{Enabled: true, Home: "~/.claude"},
	}, "", nil, nil)

	if got := reg.Resolve(claudePane()); got != "" {
		t.Fatalf("master switch off should disable all integrations, got %q", got)
	}
}

func TestBuildRegistryClaudeDisabled(
	t *testing.T,
) {
	t.Parallel()

	reg := buildRegistry(config.IntegrationsConfig{
		Enabled: true,
		Claude:  config.ClaudeIntegrationConfig{Enabled: false},
	}, "", nil, nil)

	if got := reg.Resolve(claudePane()); got != "" {
		t.Fatalf("disabled claude should not resolve, got %q", got)
	}
}

func TestBuildRegistryLiveRefreshSharedAcrossAgents(t *testing.T) {
	t.Parallel()
	saved := claudePane()
	saved.Agent.Status = "error"
	codexPane := claudePane()
	codexPane.CurrentCmd = "codex"
	codexPane.Agent.Kind = "codex"
	codexPane.Agent.Argv = []string{"codex", "resume", codexPane.Agent.ID}
	liveClaude, liveCodex := claudePane(), codexPane
	liveCodex.Agent = new(snapshot.AgentSession)
	*liveCodex.Agent = *codexPane.Agent
	liveClaude.Agent.Status, liveCodex.Agent.Status = "working", "working"
	antexPane := claudePane()
	antexPane.CurrentCmd = "antex"
	antexPane.Agent.Kind = "antex"
	antexPane.Agent.Argv = []string{"antex", "resume", antexPane.Agent.ID}
	antexPane.Agent.Home = t.TempDir()
	if err := os.Mkdir(filepath.Join(antexPane.Agent.Home, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	header := `{"type":"session_meta","payload":{"id":"sess-9","cwd":"/workspace"}}` + "\n"
	if err := os.WriteFile(
		filepath.Join(antexPane.Agent.Home, "sessions", "rollout.jsonl"),
		[]byte(header),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	running := true
	registry := buildRegistry(config.IntegrationsConfig{
		Enabled: true, Claude: config.ClaudeIntegrationConfig{Enabled: true},
		Codex: config.AgentIntegrationConfig{Enabled: true},
		Antex: config.AntexIntegrationConfig{Enabled: true, Home: antexPane.Agent.Home},
	}, "", nil, func() ([]snapshot.Pane, error) {
		calls.Add(1)
		if !running {
			return nil, nil
		}

		return []snapshot.Pane{liveClaude, liveCodex, antexPane}, nil
	})
	app, _ := newTestApp(t)
	windows := []snapshot.Window{{Index: 1, Panes: []snapshot.Pane{saved, codexPane}}}
	if err := app.store.SaveSession(
		snapshot.SessionSnapshot{SessionName: "work", Windows: windows},
	); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"working", "idle", "awaiting_input"} {
		liveClaude.Agent.Status, liveCodex.Agent.Status = state, state
		scope := registry.Scope()
		var workers sync.WaitGroup
		for range 16 {
			workers.Go(func() {
				if got, ok := scope.Status(antexPane); !ok || got != integration.StatusIdle {
					t.Errorf("Antex status: %v %v", got, ok)
				}
				for _, pane := range []snapshot.Pane{saved, codexPane} {
					got, ok := scope.Status(pane)
					want, _ := agent.State(state)
					if !ok || got != want {
						t.Errorf("%s: %v %v", state, got, ok)
					}
				}
			})
		}
		workers.Wait()
		if got := app.sessionHasWorkingAgent(scope, "work", true); got != (state == "working") {
			t.Fatalf("quick picker %s = %v", state, got)
		}
	}
	running = false
	stopped := registry.Scope()
	if _, ok := stopped.Status(saved); ok {
		t.Fatal("exited Claude retained saved status")
	}
	if app.sessionHasWorkingAgent(stopped, "work", true) {
		t.Fatal("quick picker retained exited clients")
	}
	if calls.Load() != 4 {
		t.Fatalf("captures = %d, want one per refresh across all three agents", calls.Load())
	}
	stored, err := app.store.LoadSession("work")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Windows[0].Panes[0].Agent.Status != "error" {
		t.Fatal("status refresh rewrote saved snapshot")
	}
}
