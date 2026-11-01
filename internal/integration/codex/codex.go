package codex

import (
	"fmt"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type Integration struct{ live agent.LiveStatus }

func New(_ string) *Integration     { return &Integration{} }
func (i *Integration) Name() string { return "codex" }
func (i *Integration) Scope() integration.Integration {
	return i.ScopeWithStatus(&integration.StatusSnapshot{})
}
func (i *Integration) Matches(pane snapshot.Pane) bool { return agent.Matches(pane, "codex") }
func (i *Integration) Capture(pane snapshot.Pane) (map[string]string, error) {
	return agent.Capture(pane, "codex"), nil
}

func (i *Integration) RestoreCommand(pane snapshot.Pane, _ map[string]string) string {
	command, _ := agent.Restore(pane, "codex")

	return command
}

func (i *Integration) RestoreDecision(pane snapshot.Pane, _ map[string]string) (string, error) {
	command, err := agent.Restore(pane, "codex")
	if err != nil {
		return "", fmt.Errorf("restore agent: %w", err)
	}

	return command, nil
}

func (i *Integration) Status(pane snapshot.Pane) (integration.Status, bool) {
	return i.live.Status(pane, "codex")
}

func (i *Integration) WithStatusLoader(loader integration.StatusLoader) *Integration {
	i.live.Loader = loader

	return i
}

func (i *Integration) ScopeWithStatus(cache *integration.StatusSnapshot) integration.Integration {
	return &Integration{live: i.live.Scope(cache)}
}
