package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeHooksAliasInstallsLifecycleHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(home, "config.toml"))
	code, _, errOut := run(t, "claude-hooks")
	if code != 0 {
		t.Fatal(errOut)
	}
	path := filepath.Join(home, ".claude/settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "SessionEnd", "PermissionRequest"} {
		if !strings.Contains(string(data), event) {
			t.Fatal(event)
		}
	}
	if strings.Contains(string(data), "PermissionDenied") ||
		strings.Contains(string(data), "hook claude-status") {
		t.Fatal("legacy status hooks installed")
	}
	code, _, errOut = run(t, "claude-hooks", "--uninstall")
	if code != 0 {
		t.Fatal(errOut)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hook agent") {
		t.Fatal("uninstall left owned hooks")
	}
}
