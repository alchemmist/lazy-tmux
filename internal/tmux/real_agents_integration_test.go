package tmux_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestRealAgentLifecycle(t *testing.T) {
	t.Setenv("LAZY_TMUX_REAL_TEST_RUNNING", "1")
	if os.Getenv("ENABLE_REAL_AGENT_TESTS") != "true" {
		t.Skip(
			"opt-in: use make real-agent-test with dedicated client homes and pinned version strings",
		)
	}
	for _, kind := range []string{"codex", "claude"} {
		t.Run(
			kind,
			func(t *testing.T) { t.Setenv("LAZY_TMUX_REAL_TEST_RUNNING", "1"); runRealAgentLifecycle(t, kind) },
		)
	}
}

func runRealAgentLifecycle(t *testing.T, kind string) {
	t.Helper()
	prefix := "LAZY_TMUX_REAL_" + strings.ToUpper(kind)
	home := os.Getenv(prefix + "_HOME")
	version := os.Getenv(prefix + "_VERSION")
	if home == "" || version == "" {
		t.Fatalf(
			"%s_HOME and %s_VERSION are required; no credentials are read from normal client homes",
			prefix,
			prefix,
		)
	}
	marker, err := os.ReadFile(home + "/.lazy-tmux-test-home")
	if err != nil || strings.TrimSpace(string(marker)) != kind {
		t.Fatal("dedicated test home must contain .lazy-tmux-test-home naming this agent")
	}
	result, err := exec.CommandContext(context.Background(), kind, "--version").Output()
	if err != nil || strings.TrimSpace(string(result)) != version {
		t.Fatalf("pinned version mismatch: got %q wanted %q (%v)", result, version, err)
	}
	testutil.IsolatedTmux(t)
	name := "real-" + kind
	envName := "CODEX_HOME"
	if kind == "claude" {
		envName = "CLAUDE_CONFIG_DIR"
	}
	launch := "env " + agent.Quote(envName+"="+home) + " " + kind
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", t.TempDir(), launch)
	client := tmux.NewClient("tmux")
	client.SetRestoreResolver(
		integration.NewRegistry(codex.New(home), claude.New(home, t.TempDir())),
	)
	before := waitForRealBinding(t, client, name, kind)
	if err := client.KillSession(name); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSession(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	after := waitForRealBinding(t, client, name, kind)
	if after.Windows[0].Panes[0].Agent.ID != before.Windows[0].Panes[0].Agent.ID {
		t.Fatal("real client resumed a different conversation")
	}
	t.Logf("verified %s: %s", kind, version)
}

func waitForRealBinding(
	t *testing.T,
	client *tmux.Client,
	name, kind string,
) snapshot.SessionSnapshot {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		captured, err := client.CaptureSession(name)
		if err == nil && len(captured.Windows) > 0 && len(captured.Windows[0].Panes) > 0 {
			pane := captured.Windows[0].Panes[0]
			if pane.Agent != nil && pane.Agent.Kind == kind {
				return captured
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf(
		"no verified %s binding; check client login, hooks trust, version and ownership topology",
		kind,
	)

	return snapshot.SessionSnapshot{}
}
