package tmux_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

//nolint:paralleltest // Isolated tmux server and process-wide environment.
func TestRealPiRestoreRejectsReplacedHistoryBeforeStartup(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires real Pi")
	}
	testutil.IsolatedTmux(t)
	home, cwd := t.TempDir(), t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessions, "history.jsonl")
	writePiSession(t, file, "original", cwd)
	pane := snapshot.Pane{
		CurrentCmd:  "pi",
		RestoreCmd:  "pi",
		CurrentPath: cwd,
		Meta:        map[string]string{"pi.session_id": "original", "pi.session_file": file},
	}
	registry := integration.NewRegistry(pi.New(home, sessions))
	if _, err := registry.ResolveChecked(pane); err != nil {
		t.Fatal(err)
	}
	// Deterministically replace the file after restore validation, before launch.
	writePiSession(t, file, "replacement", cwd)
	intent, err := json.Marshal(map[string]any{"token": "auditToken", "pane": pane})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "pi-ack", "-c", cwd, "sleep 30")
	target := "pi-ack:0.0"
	testutil.Tmux(
		t,
		"set-option",
		"-p",
		"-t",
		target,
		"@lazy_tmux_restore_pending",
		base64.StdEncoding.EncodeToString(intent),
	)
	testutil.Tmux(t, "set-option", "-p", "-t", target, "@lazy_tmux_restore_ack", "")
	launch := "env PI_OFFLINE=1 LAZY_TMUX_RESTORE_TOKEN=auditToken PI_CODING_AGENT_DIR=" + agent.Quote(
		home,
	) +
		" PI_CODING_AGENT_SESSION_DIR=" + agent.Quote(
		sessions,
	) + " pi --no-tools --session " + agent.Quote(
		file,
	)
	testutil.Tmux(t, "respawn-pane", "-k", "-t", target, launch)
	deadline := time.Now().Add(20 * time.Second)
	for {
		raw := testutil.Tmux(t, "show-option", "-pqv", "-t", target, "@lazy_tmux_pi_binding")
		if strings.Contains(raw, `"session_id":"replacement"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Pi did not publish: %s", capturePane(t, "pi-ack"))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ack := strings.TrimSpace(
		testutil.Tmux(t, "show-option", "-pqv", "-t", target, "@lazy_tmux_restore_ack"),
	); ack != "" {
		t.Fatalf("wrong conversation acknowledged saved intent: %q", ack)
	}
	captured, err := tmux.NewClient("tmux").CaptureSession("pi-ack")
	if err != nil {
		t.Fatal(err)
	}
	registry.Enrich(&captured)
	got := captured.Windows[0].Panes[0]
	if got.Meta[agent.PendingKey] == "" || got.Meta["pi.session_id"] != "original" {
		t.Fatalf("saved intent lost: %+v", got)
	}
}
