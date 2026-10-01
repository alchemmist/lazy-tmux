package integration

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func CommandAgent(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	name := filepath.Base(fields[0])
	if slices.Contains(
		[]string{snapshot.AgentCodex, snapshot.AgentAntex, snapshot.AgentClaude},
		name,
	) {
		return name
	}
	if name == "node" && len(fields) > 1 {
		path := filepath.ToSlash(fields[1])
		if strings.Contains(path, "/@anthropic-ai/claude-code/") &&
			filepath.Base(path) == "cli.js" {
			return snapshot.AgentClaude
		}
		if strings.Contains(path, "/@openai/codex/") && filepath.Base(path) == "codex.js" {
			return snapshot.AgentCodex
		}
	}

	return ""
}
