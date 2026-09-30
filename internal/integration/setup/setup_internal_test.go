package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUninstallPreservesForeignHooksAndBackup(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"codex", "claude"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			name, err := location(kind)
			path := filepath.Join(home, name)
			if err != nil {
				t.Fatal(err)
			}
			original := `{"other":{"keep":true},"hooks":{"SessionStart":[` +
				`{"hooks":[{"type":"command","command":"echo foreign"}]}]}}`
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			binary := "/path with spaces/lazy-tmux"
			if changed, err := Apply(kind, home, binary, false); err != nil || !changed {
				t.Fatalf("install %v %v", changed, err)
			}
			if ok, err := Check(kind, home, binary); err != nil || !ok {
				t.Fatalf("check %v %v", ok, err)
			}
			if changed, err := Apply(kind, home, binary, false); err != nil || changed {
				t.Fatalf("idempotence %v %v", changed, err)
			}
			if changed, err := Apply(kind, home, binary, true); err != nil || !changed {
				t.Fatalf("uninstall %v %v", changed, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "echo foreign") ||
				strings.Contains(string(data), marker) {
				t.Fatal(string(data))
			}
			assertHookBackup(t, path, original)
		})
	}
}

func TestMalformedConfigIsNeverOverwritten(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "settings.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("claude", home, "/bin/lazy-tmux", false); err == nil {
		t.Fatal("malformed config accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{" {
		t.Fatal("malformed config overwritten")
	}
}

func assertHookBackup(t *testing.T, path, original string) {
	t.Helper()
	backup, err := os.ReadFile(path + ".lazy-tmux.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup changed: %s %v", backup, err)
	}
}

func TestUninstallKeepsForeignHooksInMixedGroup(t *testing.T) {
	t.Parallel()
	command := hookLine("claude", "/tmp/home", "/bin/lazy-tmux")
	raw, err := json.Marshal(
		map[string]any{
			"hooks": map[string]any{
				"SessionStart": []any{
					map[string]any{
						"matcher": "startup",
						"unknown": "keep",
						"hooks": []any{
							map[string]any{"type": "command", "command": command},
							map[string]any{"type": "command", "command": "foreign", "timeout": 12},
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := merge(raw, "claude", "/tmp/home", "/bin/lazy-tmux", true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), marker) || !strings.Contains(string(result), "foreign") ||
		!strings.Contains(string(result), "unknown") ||
		!strings.Contains(string(result), "timeout") {
		t.Fatal(string(result))
	}
}
