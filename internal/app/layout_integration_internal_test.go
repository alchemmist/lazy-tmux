package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestFreshPiKeepsLiveWindowsAndLastResumableSave(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires real Pi")
	}
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	a, dir := newTestApp(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	a.integrations = integration.NewRegistry(pi.New(home, ""))
	const session = "lab"
	if err := a.NewSession(session); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessions", "lab.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(
		testutil.Tmux(
			t,
			"new-window",
			"-P",
			"-F",
			"#{pane_id}",
			"-t",
			session,
			"-n",
			"fresh-pi",
			"env PI_OFFLINE=1 PI_CODING_AGENT_DIR="+home+" pi --no-tools",
		),
	)
	deadline := time.Now().Add(20 * time.Second)
	for strings.TrimSpace(testutil.Tmux(t, "display-message", "-p", "-t", pane, "#{@lazy_tmux_pi_binding}")) == "" {
		if time.Now().After(deadline) {
			t.Fatal("Pi did not publish its binding")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := a.SaveSession(session); err == nil || !strings.Contains(err.Error(), "layout saved") {
		t.Fatalf("partial save: %v", err)
	}
	if err := a.Sleep(session); err == nil {
		t.Fatal("unsafe sleep succeeded")
	}
	assertAgentSaveFileUnchanged(t, path, before)
	sessions, err := a.pickerSessions(DefaultPickerSortOptions())
	if err != nil || len(sessions) != 1 || len(sessions[0].Windows) != 2 ||
		sessions[0].Windows[1].Name != "fresh-pi" {
		t.Fatalf("live windows: %+v, %v", sessions, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "recovery", "layouts", "lab.json"))
	if err != nil || !json.Valid(data) || !bytes.Contains(data, []byte("fresh-pi")) {
		t.Fatalf("missing layout: %s %v", data, err)
	}
}
