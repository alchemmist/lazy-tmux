package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)

	ss := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "work/main",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 0, Name: "editor", Panes: []snapshot.Pane{{Index: 0}, {Index: 1}}},
		},
	}

	if err := s.SaveSession(ss); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := s.LoadSession("work/main")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.SessionName != ss.SessionName {
		t.Fatalf("expected session %q, got %q", ss.SessionName, loaded.SessionName)
	}

	recs, err := s.ListRecords()
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
	s := New(t.TempDir())
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "old",
		CapturedAt:  base,
		Windows:     []snapshot.Window{{Index: 0}},
	}); err != nil {
		t.Fatalf("save old: %v", err)
	}
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "new",
		CapturedAt:  base.Add(1 * time.Hour),
		Windows:     []snapshot.Window{{Index: 0}},
	}); err != nil {
		t.Fatalf("save new: %v", err)
	}

	recs, err := s.ListRecords()
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
	s := New(t.TempDir())
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  base,
		Windows:     []snapshot.Window{{Index: 0}},
	}); err != nil {
		t.Fatalf("save demo: %v", err)
	}

	accessedAt := base.Add(30 * time.Minute)
	if err := s.MarkSessionAccessed("demo", accessedAt); err != nil {
		t.Fatalf("mark accessed: %v", err)
	}

	recs, err := s.ListRecords()
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
	s := New(t.TempDir())
	ss := snapshot.SessionSnapshot{
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
	if err := s.SaveSession(ss); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := s.LoadSession("work")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Windows[0].Panes[0].Scrollback == nil {
		t.Fatal("expected scrollback metadata")
	}
	sb := loaded.Windows[0].Panes[0].Scrollback
	if sb.Ref == "" {
		t.Fatal("expected scrollback ref")
	}
	if sb.Content != "echo 1\n1\n" {
		t.Fatalf("unexpected scrollback content: %q", sb.Content)
	}
	if sb.Bytes == 0 || sb.Lines == 0 {
		t.Fatalf("expected non-zero scrollback metadata, got lines=%d bytes=%d", sb.Lines, sb.Bytes)
	}
}

func TestSaveSessionWithoutScrollbackDoesNotCreateSessionScrollbackDir(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	ss := snapshot.SessionSnapshot{
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
	if err := s.SaveSession(ss); err != nil {
		t.Fatalf("save: %v", err)
	}

	sessionDir := filepath.Join(base, scrollbackDir, sanitizeName("plain"))
	if _, err := os.Stat(sessionDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no scrollback dir, got err=%v", err)
	}
}
