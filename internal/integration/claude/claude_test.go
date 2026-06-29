package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// writeTranscript creates <home>/projects/<encoded cwd>/<id>.jsonl with the
// given modification time so tests can control which one is "newest".
func writeTranscript(t *testing.T, home, cwd, id string, mod time.Time) {
	t.Helper()

	dir := filepath.Join(home, "projects", encodeProjectDir(cwd))
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
	home := t.TempDir()
	cwd := "/Users/me/code/proj"

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTranscript(t, home, cwd, "old-session", base)
	writeTranscript(t, home, cwd, "new-session", base.Add(time.Hour))

	integ := New(home)

	meta, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if meta["session_id"] != "new-session" {
		t.Fatalf("expected newest session, got %q", meta["session_id"])
	}
}

func TestCaptureMissingProjectDir(t *testing.T) {
	integ := New(t.TempDir())

	meta, err := integ.Capture(snapshot.Pane{CurrentPath: "/no/such/dir", CurrentCmd: "claude"})
	if err != nil {
		t.Fatalf("capture should not error on missing dir: %v", err)
	}

	if len(meta) != 0 {
		t.Fatalf("missing dir should yield no meta, got %v", meta)
	}
}

func TestCaptureIgnoresNonTranscripts(t *testing.T) {
	home := t.TempDir()
	cwd := "/Users/me/proj"

	dir := filepath.Join(home, "projects", encodeProjectDir(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, _ := New(home).Capture(snapshot.Pane{CurrentPath: cwd})
	if len(meta) != 0 {
		t.Fatalf("non-.jsonl files must be ignored, got %v", meta)
	}
}

func TestMatches(t *testing.T) {
	integ := New(t.TempDir())

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
	integ := New(t.TempDir())

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
	if got := encodeProjectDir("/Users/me/code/lazy-tmux"); got != "-Users-me-code-lazy-tmux" {
		t.Fatalf("unexpected encoding: %q", got)
	}

	if got := encodeProjectDir("/a/b.c/d"); got != "-a-b-c-d" {
		t.Fatalf("dots should map to '-', got %q", got)
	}
}
