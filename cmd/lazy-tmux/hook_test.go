package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	// File is keyed by the project-dir encoding (/ and . -> -).
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
