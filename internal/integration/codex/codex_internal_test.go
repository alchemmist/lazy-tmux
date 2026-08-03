package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeRollout(t *testing.T, home, date, id, cwd string, mod time.Time) {
	t.Helper()
	path := filepath.Join(home, "sessions", date, "rollout-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"` +
		id + `","cwd":"` + cwd + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureReturnsNewestMatchingSession(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeRollout(t, home, "2026/01/01", "old", cwd, base)
	writeRollout(t, home, "2026/01/02", "other-cwd", "/tmp", base.Add(2*time.Hour))
	writeRollout(t, home, "2026/01/03", "new", cwd, base.Add(time.Hour))

	meta, err := New(home).Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "codex"})
	if err != nil || meta[metaSessionID] != "new" {
		t.Fatalf("Capture() = %v, %v; want newest matching session", meta, err)
	}
}

func TestMatchesAndRestore(t *testing.T) {
	t.Parallel()

	i := New(t.TempDir())
	if !i.Matches(snapshot.Pane{CurrentCmd: "codex"}) ||
		!i.Matches(snapshot.Pane{RestoreCmd: "codex resume abc"}) {
		t.Fatal("expected codex commands to match")
	}
	if i.Matches(snapshot.Pane{CurrentCmd: "claude"}) {
		t.Fatal("claude must not match codex integration")
	}
	if got := i.RestoreCommand(
		snapshot.Pane{},
		map[string]string{metaSessionID: "abc"},
	); got != "codex resume abc" {
		t.Fatalf("unexpected restore command %q", got)
	}
}
