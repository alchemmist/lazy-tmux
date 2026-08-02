package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

func TestRunClaudeStatusHookWritesFile(t *testing.T) {
	dataDir := t.TempDir()

	cfgPath := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	if err := os.WriteFile(cfgPath, []byte(`data_dir = "`+dataDir+`"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LAZY_TMUX_CONFIG", cfgPath)

	cwd := "/Users/me/code/proj"
	stdin := strings.NewReader(`{"session_id":"s-1","cwd":"` + cwd + `"}`)

	code := runClaudeStatusHook([]string{"--state", "awaiting_decision"}, stdin, io.Discard)
	if code != 0 {
		t.Fatalf("hook exit code = %d", code)
	}

	statusFile := filepath.Join(dataDir, "claude-status", "-Users-me-code-proj.json")

	data, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("status file not written: %v", err)
	}

	for _, want := range []string{`"state":"awaiting_decision"`, `"cwd":"` + cwd + `"`, `"session_id":"s-1"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("status file missing %q:\n%s", want, data)
		}
	}
}

func TestRunClaudeStatusHookRejectsBadState(t *testing.T) {
	t.Parallel()

	code := runClaudeStatusHook([]string{"--state", "bogus"}, strings.NewReader("{}"), io.Discard)
	if code == 0 {
		t.Fatal("invalid --state should be rejected")
	}
}

func TestRunThemeHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	t.Setenv("LAZY_TMUX_CONFIG", path)

	if code := runThemeHook([]string{"--theme", "light"}, io.Discard); code != 0 {
		t.Fatalf("theme hook exit code = %d", code)
	}

	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "light" {
		t.Fatalf("theme = %q, want light", cfg.Theme)
	}
}

func TestRunThemeHookAcceptsPositionalTheme(t *testing.T) {
	t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(t.TempDir(), "lazy-tmux.toml"))

	if code := runThemeHook([]string{"dark"}, io.Discard); code != 0 {
		t.Fatalf("theme hook exit code = %d", code)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Theme != "dark" {
		t.Fatalf("theme = %q, want dark", cfg.Theme)
	}
}
