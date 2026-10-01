package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCommandsRejectInvalidTargets(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"integrations", "nonsense"},
		{"integrations", "setup"},
		{"integrations", "repair"},
		{"agent-session", "--help"},
	} {
		code, _, errOut := run(t, args...)
		if args[0] == "agent-session" {
			if code != 0 {
				t.Fatal(errOut)
			}

			continue
		}
		if code == 0 {
			t.Fatalf("invalid target accepted: %v", args)
		}
	}
}

func TestIntegrationSetupCLIPreservesClientConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(home, "config.toml"))
	code, out, errOut := run(t, "integrations", "setup", "codex")
	if code != 0 || !strings.Contains(out, "trust") {
		t.Fatalf("setup: %d %s %s", code, out, errOut)
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex/hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SessionStart") ||
		strings.Contains(string(data), "trust=true") {
		t.Fatal(string(data))
	}
	code, _, errOut = run(t, "integrations", "setup", "codex", "--uninstall")
	if code != 0 {
		t.Fatal(errOut)
	}
}
