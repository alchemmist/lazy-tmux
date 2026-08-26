package claude

import (
	"os"
	"path/filepath"
	"strconv"
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

func writeLiveSession(t *testing.T, home string, pid int, sessionID, cwd string) {
	t.Helper()

	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	body := `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"` + sessionID + `","cwd":"` + cwd + `"}`

	path := filepath.Join(dir, strconv.Itoa(pid)+".json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write live session: %v", err)
	}
}

func TestCaptureDisambiguatesPanesInSameDirectory(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"

	// Same directory, single transcript on disk — latestSessionID alone
	// can't tell the two panes apart, both would resolve to this one.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTranscript(t, home, cwd, "either-pane-session", base)

	writeLiveSession(t, home, 1001, "session-a", cwd)
	writeLiveSession(t, home, 1002, "session-b", cwd)

	integ := New(home, "")

	metaA, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude", ForegroundPID: 1001})
	if err != nil {
		t.Fatalf("capture pane a: %v", err)
	}

	metaB, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude", ForegroundPID: 1002})
	if err != nil {
		t.Fatalf("capture pane b: %v", err)
	}

	if metaA["session_id"] != "session-a" {
		t.Fatalf("pane a: expected session-a, got %q", metaA["session_id"])
	}

	if metaB["session_id"] != "session-b" {
		t.Fatalf("pane b: expected session-b, got %q", metaB["session_id"])
	}
}

func TestCaptureFallsBackWhenNoLiveSessionFile(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTranscript(t, home, cwd, "from-directory", base)

	integ := New(home, "")

	// No ~/.claude/sessions/9999.json — must fall back to the directory scan.
	meta, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude", ForegroundPID: 9999})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if meta["session_id"] != "from-directory" {
		t.Fatalf("expected fallback to directory scan, got %q", meta["session_id"])
	}
}

func TestCaptureIgnoresLiveSessionForDifferentCwd(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTranscript(t, home, cwd, "from-directory", base)

	// A reused/stale pid whose session file points at a different cwd
	// entirely — must not be trusted, falls back to the directory scan.
	writeLiveSession(t, home, 1003, "stale-session", "/Users/me/code/other-proj")

	integ := New(home, "")

	meta, err := integ.Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "claude", ForegroundPID: 1003})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	if meta["session_id"] != "from-directory" {
		t.Fatalf("cwd mismatch should be ignored, got %q", meta["session_id"])
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
