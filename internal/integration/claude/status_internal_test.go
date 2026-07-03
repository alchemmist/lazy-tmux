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
	t.Parallel()

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
	t.Parallel()

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

func TestStatusHookIgnoresCWDMismatch(t *testing.T) {
	t.Parallel()

	statusDir := t.TempDir()
	pane := "/Users/me/code/proj"

	// A hook file sitting at this pane's encoded path but recording a different
	// cwd (an encoding collision) must not leak another project's status.
	body := `{"state":"working","cwd":"/some/other/project"}`
	path := filepath.Join(statusDir, EncodeProjectDir(pane)+".json")

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ok := New("", statusDir).Status(snapshot.Pane{CurrentPath: pane}); ok {
		t.Fatal("hook status with a mismatched cwd must be ignored")
	}
}

func TestStatusFallsBackToSessionFile(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/proj"

	// Both alive (use this process's pid); the freshest wins.
	writeSessionFile(t, home, "busy", cwd, "busy", 200)
	writeSessionFile(t, home, "idle", cwd, "idle", 100) // older, ignored

	got, ok := New(home, t.TempDir()).Status(snapshot.Pane{CurrentPath: cwd})
	if !ok || got != integration.StatusWorking {
		t.Fatalf("expected working from freshest live session file, got %v ok=%v", got, ok)
	}
}

func TestStatusSessionFileWaiting(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/proj"

	writeSessionFile(t, home, "w", cwd, "waiting", 300)

	got, ok := New(home, t.TempDir()).Status(snapshot.Pane{CurrentPath: cwd})
	if !ok || got != integration.StatusAwaitingDecision {
		t.Fatalf("expected awaiting-decision from a waiting session, got %v ok=%v", got, ok)
	}
}

func TestStatusSessionFileSkipsDeadProcess(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/proj"

	// A dead pid must be ignored so a stale file never shows a dot.
	writeSessionFilePID(t, home, "dead", 2147483600, cwd, "busy", 999)

	if _, ok := New(home, t.TempDir()).Status(snapshot.Pane{CurrentPath: cwd}); ok {
		t.Fatal("dead session process must yield no status")
	}
}

func TestStatusNoneWhenUnknown(t *testing.T) {
	t.Parallel()

	got, ok := New(t.TempDir(), t.TempDir()).Status(snapshot.Pane{CurrentPath: "/nowhere"})
	if ok || got != integration.StatusUnknown {
		t.Fatalf("expected no status, got %v ok=%v", got, ok)
	}

	if _, ok := New("", "").Status(snapshot.Pane{CurrentPath: ""}); ok {
		t.Fatal("empty cwd should yield no status")
	}
}

// writeSessionFile writes a Claude session file with this (alive) test process's
// pid, so the liveness check passes.
func writeSessionFile(t *testing.T, home, name, cwd, status string, updatedAt int64) {
	t.Helper()
	writeSessionFilePID(t, home, name, os.Getpid(), cwd, status, updatedAt)
}

func writeSessionFilePID(
	t *testing.T,
	home, name string,
	pid int,
	cwd, status string,
	updatedAt int64,
) {
	t.Helper()

	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	body := `{"pid":` + strconv.Itoa(pid) + `,"cwd":"` + cwd + `","status":"` + status +
		`","updatedAt":` + strconv.FormatInt(updatedAt, 10) + `}`
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
