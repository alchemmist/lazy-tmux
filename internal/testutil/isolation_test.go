package testutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

//nolint:paralleltest // The test configures the environment inherited by tmux.
func TestIsolatedServerDoesNotInheritUserPaths(t *testing.T) {
	keys := []string{
		"HOME",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"XDG_STATE_HOME",
		"XDG_CACHE_HOME",
		"XDG_RUNTIME_DIR",
		"LAZY_TMUX_DATA_DIR",
		"LAZY_TMUX_CONFIG",
		"ANTEX_HOME",
		"CODEX_HOME",
		"CLAUDE_CONFIG_DIR",
		"PI_CODING_AGENT_DIR",
		"PI_CODING_AGENT_SESSION_DIR",
	}
	inherited := t.TempDir()
	for _, key := range keys {
		t.Setenv(key, inherited)
	}
	testutil.IsolatedTmux(t)
	home := os.Getenv("HOME")
	if home == inherited {
		t.Fatal("test inherited HOME")
	}
	for _, key := range keys {
		value := os.Getenv(key)
		rel, err := filepath.Rel(home, value)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("%s is outside temporary HOME: %q", key, value)
		}
		if got := strings.TrimSpace(testutil.Tmux(t, "show-environment",
			"-g", key)); got != key+"="+value {
			t.Fatalf("server %s; expected %s=%s", got, key, value)
		}
	}
}
