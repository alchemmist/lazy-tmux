package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeTranscript(t *testing.T, home, cwd, id string, mod time.Time) {
	t.Helper()

	dir := filepath.Join(home, "projects", EncodeProjectDir(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	path := filepath.Join(dir, id+transcriptExt)
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestCaptureReturnsNewestSession(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTranscript(t, home, cwd, "old-session", base)
	writeTranscript(t, home, cwd, "new-session", base.Add(time.Hour))

	integ := New(home, "")

	meta, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if meta["session_id"] != "new-session" {
		t.Fatalf("expected newest session, got %q", meta["session_id"])
	}
}

func TestCaptureInvalidatesIndexWhenExistingTranscriptChanges(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"
	base := time.Unix(100, 0)
	writeTranscript(t, home, cwd, "old", base)
	writeTranscript(t, home, cwd, "new", base.Add(time.Hour))
	integration := New(home, "")
	pane := snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude"}

	meta, err := integration.Capture(pane)
	if err != nil || meta["session_id"] != "new" {
		t.Fatalf("initial capture: meta=%v err=%v", meta, err)
	}
	oldPath := filepath.Join(home, "projects", EncodeProjectDir(cwd), "old"+transcriptExt)
	if err = os.Chtimes(oldPath, base.Add(2*time.Hour), base.Add(2*time.Hour)); err != nil {
		t.Fatalf("update old transcript: %v", err)
	}
	meta, err = integration.Capture(pane)
	if err != nil || meta["session_id"] != "old" {
		t.Fatalf("capture after update: meta=%v err=%v", meta, err)
	}
}

func TestCaptureConcurrentReadersBuildOneProjectIndex(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/shared"
	writeTranscript(t, home, cwd, "session", time.Now())
	integration := New(home, "")

	errs := make(chan string, 32)
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			meta, err := integration.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude"})
			if err != nil || meta["session_id"] != "session" {
				errs <- fmt.Sprintf("meta=%v err=%v", meta, err)
			}
		})
	}
	group.Wait()
	close(errs)
	for result := range errs {
		t.Fatal(result)
	}
	if integration.indexBuilds != 1 {
		t.Fatalf("project directory scanned %d times, want 1", integration.indexBuilds)
	}
}

func TestCaptureMissingProjectDir(t *testing.T) {
	t.Parallel()

	integ := New(t.TempDir(), "")

	meta, err := integ.Capture(snapshot.Pane{CurrentPath: "/no/such/dir", CurrentCmd: "claude"})
	if err != nil {
		t.Fatalf("capture should not error on missing dir: %v", err)
	}

	if len(meta) != 0 {
		t.Fatalf("missing dir should yield no meta, got %v", meta)
	}
}

func TestCaptureIgnoresNonTranscripts(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/proj"

	dir := filepath.Join(home, "projects", EncodeProjectDir(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, _ := New(home, "").Capture(snapshot.Pane{CurrentPath: cwd})
	if len(meta) != 0 {
		t.Fatalf("non-.jsonl files must be ignored, got %v", meta)
	}
}

func TestMatches(t *testing.T) {
	t.Parallel()

	integ := New(t.TempDir(), "")

	cases := []struct {
		pane snapshot.Pane
		want bool
	}{
		{snapshot.Pane{CurrentCmd: "claude"}, true},
		{snapshot.Pane{RestoreCmd: "node /usr/local/lib/claude/cli.js"}, true},
		{snapshot.Pane{CurrentCmd: "node", RestoreCmd: "claude --resume x"}, true},
		{snapshot.Pane{CurrentCmd: "vim"}, false},
		{snapshot.Pane{CurrentCmd: "zsh"}, false},
		{snapshot.Pane{}, false},
	}

	for _, tc := range cases {
		if got := integ.Matches(tc.pane); got != tc.want {
			t.Fatalf("Matches(%+v) = %v, want %v", tc.pane, got, tc.want)
		}
	}
}

func TestRestoreCommand(t *testing.T) {
	t.Parallel()

	integ := New(t.TempDir(), "")

	if got := integ.RestoreCommand(
		snapshot.Pane{},
		map[string]string{"session_id": "abc"},
	); got != "claude --resume abc" {
		t.Fatalf("unexpected restore command: %q", got)
	}

	if got := integ.RestoreCommand(snapshot.Pane{}, map[string]string{}); got != "" {
		t.Fatalf("missing session id should fall back to empty, got %q", got)
	}
}

func TestEncodeProjectDir(t *testing.T) {
	t.Parallel()

	if got := EncodeProjectDir("/Users/me/code/lazy-tmux"); got != "-Users-me-code-lazy-tmux" {
		t.Fatalf("unexpected encoding: %q", got)
	}

	if got := EncodeProjectDir("/a/b.c/d"); got != "-a-b-c-d" {
		t.Fatalf("dots should map to '-', got %q", got)
	}
}
