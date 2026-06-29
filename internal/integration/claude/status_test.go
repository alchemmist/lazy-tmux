package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestStatusFromHookFile(t *testing.T) {
	statusDir := t.TempDir()
	cwd := "/Users/me/code/proj"

	if err := WriteStatus(
		statusDir,
		cwd,
		StateAwaitingDecision,
		"sess-1",
		time.Unix(100, 0),
	); err != nil {
		t.Fatalf("write status: %v", err)
	}

	integ := New("", statusDir)

	got, ok := integ.Status(snapshot.Pane{CurrentPath: cwd})
	if !ok || got != integration.StatusAwaitingDecision {
		t.Fatalf("expected awaiting-decision, got %v ok=%v", got, ok)
	}
}

func TestStatusHookStatesMap(t *testing.T) {
	cases := map[string]integration.Status{
		StateWorking:          integration.StatusWorking,
		StateAwaitingDecision: integration.StatusAwaitingDecision,
		StateAwaitingInput:    integration.StatusAwaitingInput,
		StateIdle:             integration.StatusIdle,
	}

	for state, want := range cases {
		dir := t.TempDir()
		cwd := "/w/" + state

		if err := WriteStatus(dir, cwd, state, "", time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}

		got, ok := New("", dir).Status(snapshot.Pane{CurrentPath: cwd})
		if !ok || got != want {
			t.Fatalf("state %q -> %v ok=%v, want %v", state, got, ok, want)
		}
	}
}

func TestStatusFallsBackToSessionFile(t *testing.T) {
	home := t.TempDir()
	cwd := "/Users/me/proj"

	writeSessionFile(t, home, "111", cwd, "busy", 200)
	writeSessionFile(t, home, "222", cwd, "idle", 100) // older, ignored

	// No hook status dir -> fall back to the freshest session file (busy).
	got, ok := New(home, t.TempDir()).Status(snapshot.Pane{CurrentPath: cwd})
	if !ok || got != integration.StatusWorking {
		t.Fatalf("expected working from busy session file, got %v ok=%v", got, ok)
	}
}

func TestStatusNoneWhenUnknown(t *testing.T) {
	got, ok := New(t.TempDir(), t.TempDir()).Status(snapshot.Pane{CurrentPath: "/nowhere"})
	if ok || got != integration.StatusUnknown {
		t.Fatalf("expected no status, got %v ok=%v", got, ok)
	}

	if _, ok := New("", "").Status(snapshot.Pane{CurrentPath: ""}); ok {
		t.Fatal("empty cwd should yield no status")
	}
}

func writeSessionFile(t *testing.T, home, pid, cwd, status string, updatedAt int64) {
	t.Helper()

	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	body := `{"cwd":"` + cwd + `","status":"` + status + `","updated_at":` +
		strconv.FormatInt(updatedAt, 10) + `}`
	if err := os.WriteFile(filepath.Join(dir, pid+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
