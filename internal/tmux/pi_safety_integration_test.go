package tmux_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

//nolint:paralleltest // Uses an isolated tmux server and process-wide test environment.
func TestRealPiRPCChildPreservesInteractiveBinding(t *testing.T) {
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
	parentFile, childFile := filepath.Join(
		sessions,
		"parent.jsonl",
	), filepath.Join(
		sessions,
		"child.jsonl",
	)
	writePiSession(t, parentFile, "parent", cwd)
	writePiSession(t, childFile, "child", cwd)
	testutil.Tmux(t, "new-session", "-d", "-s", "pi-parent", "-c", cwd,
		"env PI_OFFLINE=1 PI_CODING_AGENT_DIR="+agent.Quote(home)+
			" PI_CODING_AGENT_SESSION_DIR="+agent.Quote(sessions)+
			" pi --no-tools --session "+agent.Quote(parentFile))
	client := tmux.NewClient("tmux")
	registry := integration.NewRegistry(pi.New(home, sessions))
	waitForPiSnapshot(t, client, registry, "pi-parent", parentFile, "parent")
	target := "pi-parent:0.0"
	before := testutil.Tmux(t, "show-option", "-p", "-v", "-t", target, "@lazy_tmux_pi_binding")
	paneID := strings.TrimSpace(
		testutil.Tmux(t, "display-message", "-p", "-t", target, "#{pane_id}"),
	)
	socket := strings.TrimSpace(
		testutil.Tmux(t, "display-message", "-p", "-t", target, "#{socket_path}"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pi", "--mode", "rpc", "--no-tools", "--session", childFile)
	cmd.Dir = cwd
	cmd.Env = append(
		os.Environ(),
		"PI_OFFLINE=1",
		"PI_CODING_AGENT_DIR="+home,
		"PI_CODING_AGENT_SESSION_DIR="+sessions,
		"TMUX="+socket+",0,0",
		"TMUX_PANE="+paneID,
	)
	cmd.Stdin = strings.NewReader("{\"id\":\"state\",\"type\":\"get_state\"}\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("RPC child: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"type":"response"`) {
		t.Fatalf("RPC did not initialize: %s", output)
	}
	after := testutil.Tmux(t, "show-option", "-p", "-v", "-t", target, "@lazy_tmux_pi_binding")
	if after != before {
		t.Fatalf("RPC child replaced parent binding:\nbefore %s\nafter %s", before, after)
	}
	waitForPiSnapshot(t, client, registry, "pi-parent", parentFile, "parent")
	// Linked windows list the same pane more than once, not multiple clients.
	testutil.Tmux(t, "new-session", "-d", "-s", "pi-linked", "sleep 30")
	testutil.Tmux(t, "link-window", "-s", "pi-parent:0", "-t", "pi-linked:1")
	live, err := client.CapturePiPanes()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, pane := range live {
		if pane.LiveStatusID == "parent" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("linked Pi pane counted as %d clients", count)
	}
	// Tampering with a live file's header must not silently replace saved ID.
	writePiSession(t, parentFile, "replacement", cwd)
	captured, err := client.CaptureSession("pi-parent")
	if err != nil {
		t.Fatal(err)
	}
	registry.Enrich(&captured)
	if captured.Windows[0].Panes[0].LiveStatusID != "parent" {
		t.Fatal("changed file lost the authoritative owner ID, allowing argv fallback")
	}
	if got := captured.Windows[0].Panes[0].Meta["pi.session_id"]; got != "" {
		t.Fatalf("changed live header accepted as %q", got)
	}
	// Keep the fixture valid for Pi shutdown.
	writePiSession(t, parentFile, "parent", cwd)
	var owner map[string]any
	if err := json.Unmarshal([]byte(before), &owner); err != nil {
		t.Fatal(err)
	}
	if owner["session_id"] != "parent" {
		t.Fatalf("missing live identity: %s", before)
	}
}
