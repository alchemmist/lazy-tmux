package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	store := New(dir)

	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "work/main",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 0, Name: "editor", Panes: []snapshot.Pane{{Index: 0}, {Index: 1}}},
		},
	}

	err := store.SaveSession(sessionSnapshot)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.LoadSession("work/main")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.SessionName != sessionSnapshot.SessionName {
		t.Fatalf("expected session %q, got %q", sessionSnapshot.SessionName, loaded.SessionName)
	}

	recs, err := store.ListRecords()
	if err != nil {
		t.Fatalf("list records: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	if recs[0].Panes != 2 {
		t.Fatalf("expected 2 panes, got %d", recs[0].Panes)
	}
}

func TestSanitizeName(t *testing.T) {
	got := sanitizeName("proj/main:dev")
	if got != "proj_main_dev" {
		t.Fatalf("unexpected sanitized name: %q", got)
	}
}

func TestSessionPath(t *testing.T) {
	s := New("/tmp/lazy")
	path := s.sessionPath("a b")
	want := filepath.Join("/tmp/lazy", sessionsDirName, "a_b.json")

	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
	}
}

func TestSaveSessionEmptyName(t *testing.T) {
	s := New(t.TempDir())

	err := s.SaveSession(snapshot.SessionSnapshot{})
	if err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestLatestRecordNoData(t *testing.T) {
	s := New(t.TempDir())

	_, err := s.LatestRecord()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestListRecordsSortedByCapturedAtDesc(t *testing.T) {
	store := New(t.TempDir())
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	err := store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "old",
		CapturedAt:  base,
		Windows:     []snapshot.Window{{Index: 0}},
	})
	if err != nil {
		t.Fatalf("save old: %v", err)
	}

	err = store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "new",
		CapturedAt:  base.Add(1 * time.Hour),
		Windows:     []snapshot.Window{{Index: 0}},
	})
	if err != nil {
		t.Fatalf("save new: %v", err)
	}

	recs, err := store.ListRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}

	if recs[0].SessionName != "new" || recs[1].SessionName != "old" {
		t.Fatalf("unexpected order: %#v", recs)
	}
}

func TestDefaultDataDirEnvOverride(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/tmp/custom-lazy")

	if got := DefaultDataDir(); got != "/tmp/custom-lazy" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestMarkSessionAccessed(t *testing.T) {
	store := New(t.TempDir())
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	err := store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  base,
		Windows:     []snapshot.Window{{Index: 0}},
	})
	if err != nil {
		t.Fatalf("save demo: %v", err)
	}

	accessedAt := base.Add(30 * time.Minute)

	err = store.MarkSessionAccessed("demo", accessedAt)
	if err != nil {
		t.Fatalf("mark accessed: %v", err)
	}

	recs, err := store.ListRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	if !recs[0].LastAccessed.Equal(accessedAt) {
		t.Fatalf("unexpected last_accessed: got %v want %v", recs[0].LastAccessed, accessedAt)
	}
}

func TestSaveAndLoadSessionWithScrollbackSidecar(t *testing.T) {
	store := New(t.TempDir())
	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "work",
		CapturedAt:  time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{
						Index:      1,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{
							Content: "echo 1\n1\n",
						},
					},
				},
			},
		},
	}

	err := store.SaveSession(sessionSnapshot)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.LoadSession("work")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Windows[0].Panes[0].Scrollback == nil {
		t.Fatal("expected scrollback metadata")
	}

	scrollback := loaded.Windows[0].Panes[0].Scrollback
	if scrollback.Ref == "" {
		t.Fatal("expected scrollback ref")
	}

	if scrollback.Content != "echo 1\n1\n" {
		t.Fatalf("unexpected scrollback content: %q", scrollback.Content)
	}

	if scrollback.Bytes == 0 || scrollback.Lines == 0 {
		t.Fatalf(
			"expected non-zero scrollback metadata, got lines=%d bytes=%d",
			scrollback.Lines,
			scrollback.Bytes,
		)
	}
}

func TestSaveSessionWithoutScrollbackDoesNotCreateSessionScrollbackDir(t *testing.T) {
	base := t.TempDir()
	store := New(base)
	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "plain",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "zsh"},
				},
			},
		},
	}

	err := store.SaveSession(sessionSnapshot)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	sessionDir := filepath.Join(base, scrollbackDir, sanitizeName("plain"))

	_, err = os.Stat(sessionDir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no scrollback dir, got err=%v", err)
	}
}

func TestLoadSessionRejectsScrollbackPathTraversal(t *testing.T) {
	base := t.TempDir()
	store := New(base)
	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "evil",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{
						Index:      0,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{
							Ref: "../../../etc/passwd",
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(sessionSnapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(store.sessionPath("evil")), 0o755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = os.WriteFile(store.sessionPath("evil"), jsonData, 0o644)
	if err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	_, err = store.LoadSession("evil")
	if err == nil {
		t.Fatal("expected traversal validation error")
	}

	if !strings.Contains(err.Error(), "invalid scrollback ref outside base dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSessionRejectsScrollbackSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	store := New(base)

	outsideDir := t.TempDir()

	outsideFile := filepath.Join(outsideDir, "outside.log")

	err := os.WriteFile(outsideFile, []byte("secret\n"), 0o644)
	if err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkDir := filepath.Join(base, scrollbackDir, sanitizeName("evil"))

	err = os.MkdirAll(linkDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir link dir: %v", err)
	}

	linkPath := filepath.Join(linkDir, "w0_p0.log")

	err = os.Symlink(outsideFile, linkPath)
	if err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "evil",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{
						Index:      0,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{
							Ref: filepath.Join(scrollbackDir, sanitizeName("evil"), "w0_p0.log"),
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(sessionSnapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(store.sessionPath("evil")), 0o755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = os.WriteFile(store.sessionPath("evil"), jsonData, 0o644)
	if err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	_, err = store.LoadSession("evil")
	if err == nil {
		t.Fatal("expected symlink escape validation error")
	}

	if !strings.Contains(err.Error(), "invalid scrollback ref outside base dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSessionAllowsDotDotPrefixSegmentName(t *testing.T) {
	base := t.TempDir()
	store := New(base)

	scrollDir := filepath.Join(base, scrollbackDir, "..cache")

	err := os.MkdirAll(scrollDir, 0o755)
	if err != nil {
		t.Fatalf("mkdir scroll dir: %v", err)
	}

	logPath := filepath.Join(scrollDir, "w0_p0.log")

	err = os.WriteFile(logPath, []byte("ok\n"), 0o600)
	if err != nil {
		t.Fatalf("write log: %v", err)
	}

	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{
						Index:      0,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{
							Ref: filepath.Join(scrollbackDir, "..cache", "w0_p0.log"),
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(sessionSnapshot)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	err = os.MkdirAll(filepath.Dir(store.sessionPath("demo")), 0o755)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = os.WriteFile(store.sessionPath("demo"), jsonData, 0o644)
	if err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	loaded, err := store.LoadSession("demo")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Windows[0].Panes[0].Scrollback == nil ||
		loaded.Windows[0].Panes[0].Scrollback.Content != "ok\n" {
		t.Fatalf("unexpected scrollback content: %#v", loaded.Windows[0].Panes[0].Scrollback)
	}
}

func TestSaveSessionRejectsInvalidScrollbackSessionName(t *testing.T) {
	store := New(t.TempDir())
	sessionSnapshot := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "..",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{
						Index:      0,
						CurrentCmd: "zsh",
						Scrollback: &snapshot.ScrollbackRef{Content: "x\n"},
					},
				},
			},
		},
	}

	err := store.SaveSession(sessionSnapshot)
	if err == nil {
		t.Fatal("expected invalid session name error")
	}

	if !strings.Contains(err.Error(), "invalid session name for scrollback") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSessionRemovesIndexEntry(t *testing.T) {
	store := New(t.TempDir())

	err := store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	err = store.DeleteSession("demo")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	recs, err := store.ListRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(recs) != 0 {
		t.Fatalf("expected no records, got %d", len(recs))
	}
}

func TestSessionPathEmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := New(tmpDir)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", true},
		{"   ", true},
		{"valid-session", false},
	}

	for _, caseItem := range tests {
		path, err := store.SessionPath(caseItem.name)
		if (err != nil) != caseItem.wantErr {
			t.Fatalf("SessionPath(%q) error = %v, wantErr %v", caseItem.name, err, caseItem.wantErr)
		}

		if !caseItem.wantErr {
			if path == "" {
				t.Fatalf("SessionPath(%q) returned empty path", caseItem.name)
			}

			expectedBase := "valid-session.json"
			if filepath.Base(path) != expectedBase {
				t.Fatalf("SessionPath(%q) = %q, want base %q", caseItem.name, path, expectedBase)
			}
		}
	}
}

func TestSessionExistsEmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := New(tmpDir)

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", true},
		{"   ", true},
		{"valid-session", false},
	}

	for _, caseItem := range tests {
		exists, err := store.SessionExists(caseItem.name)
		if (err != nil) != caseItem.wantErr {
			t.Fatalf("SessionExists(%q) error = %v, wantErr %v", caseItem.name, err, caseItem.wantErr)
		}

		if !caseItem.wantErr && exists {
			t.Fatalf("SessionExists(%q) = true, want false for non-existent session", caseItem.name)
		}
	}
}
