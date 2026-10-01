package app

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
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
	}, "")

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
	}, "")

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
	}, "")

	if got := reg.Resolve(claudePane()); got != "" {
		t.Fatalf("disabled claude should not resolve, got %q", got)
	}
}
