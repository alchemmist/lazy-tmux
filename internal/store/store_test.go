package store

import (
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
