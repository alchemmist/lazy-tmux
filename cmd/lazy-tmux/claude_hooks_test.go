package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// existing settings with a user's own Notification hook that must be preserved.
const existingSettings = `{
  "theme": "dark",
  "hooks": {
    "Notification": [
      {
        "matcher": "permission_prompt",
        "hooks": [{"type": "command", "command": "my-notify.sh"}]
      }
    ]
  }
}`

func writeSettings(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	return path
}

func parseSettings(t *testing.T, path string) map[string][]json.RawMessage {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var root struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	return root.Hooks
}

func countOurHooks(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return strings.Count(string(data), claudeHookCommandMarker)
}

func TestApplyClaudeHooksInstallPreservesExisting(t *testing.T) {
	t.Parallel()

	path := writeSettings(t, existingSettings)

	changed, err := applyClaudeHooks(path, "/usr/local/bin/lazy-tmux", false)
	if err != nil || !changed {
		t.Fatalf("install: changed=%v err=%v", changed, err)
	}

	hooks := parseSettings(t, path)

	// All hooked events present.
	for _, event := range []string{
		"Notification", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop",
	} {
		if len(hooks[event]) == 0 {
			t.Fatalf("event %q missing after install", event)
		}
	}

	// The user's own notify hook survived (Notification now has their group + ours).
	if !strings.Contains(string(mustJSON(t, hooks["Notification"])), "my-notify.sh") {
		t.Fatal("existing notify hook was lost")
	}

	// Top-level keys preserved.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"theme": "dark"`) {
		t.Fatal("unrelated top-level key was lost")
	}

	// Backup written.
	if _, err := os.Stat(path + ".lazy-tmux.bak"); err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
}

func TestApplyClaudeHooksIdempotent(t *testing.T) {
	t.Parallel()

	path := writeSettings(t, existingSettings)

	if _, err := applyClaudeHooks(path, "/bin/lazy-tmux", false); err != nil {
		t.Fatal(err)
	}

	first := countOurHooks(t, path)

	changed, err := applyClaudeHooks(path, "/bin/lazy-tmux", false)
	if err != nil {
		t.Fatal(err)
	}

	if changed {
		t.Fatal("second identical install should report no change")
	}

	if second := countOurHooks(t, path); second != first {
		t.Fatalf("install duplicated hooks: %d -> %d", first, second)
	}
}

func TestApplyClaudeHooksUninstallIsSurgical(t *testing.T) {
	t.Parallel()

	path := writeSettings(t, existingSettings)

	if _, err := applyClaudeHooks(path, "/bin/lazy-tmux", false); err != nil {
		t.Fatal(err)
	}

	if _, err := applyClaudeHooks(path, "/bin/lazy-tmux", true); err != nil {
		t.Fatal(err)
	}

	if n := countOurHooks(t, path); n != 0 {
		t.Fatalf("uninstall left %d of our hooks", n)
	}

	// The user's hook must remain.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "my-notify.sh") {
		t.Fatal("uninstall removed the user's own hook")
	}
}

func TestApplyClaudeHooksUninstallNoFileIsNoOp(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "settings.json")

	changed, err := applyClaudeHooks(path, "/bin/lazy-tmux", true)
	if err != nil {
		t.Fatal(err)
	}

	if changed {
		t.Fatal("uninstall with no settings file should be a no-op")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("uninstall must not create a settings file")
	}
}

func TestApplyClaudeHooksUninstallKeepsSiblingInGroup(t *testing.T) {
	t.Parallel()

	// A user command sharing the same group as ours must survive uninstall.
	mixed := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {"type": "command", "command": "/bin/lazy-tmux hook claude-status --state working"},
          {"type": "command", "command": "user-thing.sh"}
        ]
      }
    ]
  }
}`
	path := writeSettings(t, mixed)

	if _, err := applyClaudeHooks(path, "/bin/lazy-tmux", true); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if n := countOurHooks(t, path); n != 0 {
		t.Fatalf("uninstall left %d of our hooks", n)
	}

	if !strings.Contains(string(data), "user-thing.sh") {
		t.Fatal("uninstall removed a sibling command from the shared group")
	}
}

func TestApplyClaudeHooksCreatesMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "settings.json")

	changed, err := applyClaudeHooks(path, "/bin/lazy-tmux", false)
	if err != nil || !changed {
		t.Fatalf("install into new file: changed=%v err=%v", changed, err)
	}

	if countOurHooks(t, path) == 0 {
		t.Fatal("expected hooks written to fresh settings file")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
