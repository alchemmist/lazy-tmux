package claude

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestClaudeConversationsAreIndependent(t *testing.T) {
	t.Parallel()
	registry := integration.NewRegistry(New(t.TempDir(), t.TempDir()))
	state := snapshot.SessionSnapshot{Windows: []snapshot.Window{{Panes: []snapshot.Pane{
		{
			CurrentCmd:  "claude",
			CurrentPath: "/workspace",
			Agent: &snapshot.AgentSession{
				Version: 1,
				Kind:    "claude",
				ID:      "first",
				Home:    "/home/claude",
				CWD:     "/workspace",
				Argv:    []string{"claude", "--resume", "first"},
				Source:  "hook-v1",
				Status:  "idle",
			},
		},
		{
			CurrentCmd:  "claude",
			CurrentPath: "/workspace",
			Agent: &snapshot.AgentSession{
				Version: 1,
				Kind:    "claude",
				ID:      "second",
				Home:    "/home/claude",
				CWD:     "/workspace",
				Argv:    []string{"claude", "--resume", "second"},
				Source:  "hook-v1",
				Status:  "working",
			},
		},
	}}}}
	registry.Enrich(&state)
	first := state.Windows[0].Panes[0]
	second := state.Windows[0].Panes[1]
	if got := registry.Resolve(
		first,
	); got != "env 'CLAUDE_CONFIG_DIR=/home/claude' 'claude' '--resume' 'first'" {
		t.Fatal(got)
	}
	if status, ok := registry.Status(first); !ok || status != integration.StatusIdle {
		t.Fatalf("first status: %v %v", status, ok)
	}
	if status, ok := registry.Status(second); !ok || status != integration.StatusWorking {
		t.Fatalf("second status: %v %v", status, ok)
	}
}

func TestClaudeRefusesUnverifiedLegacyIdentity(t *testing.T) {
	t.Parallel()
	p := snapshot.Pane{
		CurrentCmd:  "claude",
		CurrentPath: "/workspace",
		RestoreCmd:  "claude --resume first",
		Meta:        map[string]string{"claude.session_id": "second"},
	}
	c := New(t.TempDir(), t.TempDir())
	if command, err := c.RestoreDecision(p, p.Meta); command != "" || err == nil {
		t.Fatalf("unsafe restore %q %v", command, err)
	}
	if _, ok := c.Status(p); ok {
		t.Fatal("unverified status accepted")
	}
}
