package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestConcurrentStoreInstancesPreserveEveryIndexEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stores := []*Store{New(dir), New(dir)}
	const sessionCount = 40

	errs := make(chan error, sessionCount)
	var group sync.WaitGroup
	for index := range sessionCount {
		group.Go(func() {
			name := fmt.Sprintf("session-%02d", index)
			errs <- stores[index%len(stores)].SaveSession(snapshot.SessionSnapshot{
				Version:     snapshot.FormatVersion,
				SessionName: name,
			})
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent save: %v", err)
		}
	}

	records, err := New(dir).ListRecords()
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != sessionCount {
		t.Fatalf("index has %d sessions, want %d", len(records), sessionCount)
	}
}

func TestConcurrentStoreInstancesKeepOneSessionConsistent(t *testing.T) {
	t.Parallel()

	for iteration := range 20 {
		dir := t.TempDir()
		stores := []*Store{New(dir), New(dir)}
		snapshots := []snapshot.SessionSnapshot{
			concurrentSnapshot("shared", "alpha", iteration),
			concurrentSnapshot("shared", "bravo", iteration),
		}

		errs := make(chan error, len(stores))
		var group sync.WaitGroup
		for index := range stores {
			group.Go(func() { errs <- stores[index].SaveSession(snapshots[index]) })
		}
		group.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("iteration %d concurrent same-session save: %v", iteration, err)
			}
		}

		loaded, err := New(dir).LoadSession("shared")
		if err != nil {
			t.Fatalf("iteration %d load: %v", iteration, err)
		}
		marker := loaded.Windows[0].Name
		for _, pane := range loaded.Windows[0].Panes {
			if pane.Scrollback == nil || !strings.Contains(pane.Scrollback.Content, marker) {
				t.Fatalf("iteration %d mixed snapshot %s: %+v", iteration, marker, loaded)
			}
		}
	}
}

func concurrentSnapshot(name, marker string, iteration int) snapshot.SessionSnapshot {
	panes := make([]snapshot.Pane, 12)
	for index := range panes {
		panes[index] = snapshot.Pane{
			Index:       index,
			CurrentPath: "/tmp",
			CurrentCmd:  "zsh",
			RestoreCmd:  "",
			Scrollback: &snapshot.ScrollbackRef{
				Ref:     "",
				Lines:   0,
				Bytes:   0,
				Content: strings.Repeat(marker, 1024),
			},
			IsActive: index == 0,
			Meta:     nil,
		}
	}

	return snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CapturedAt:  time.Unix(int64(iteration+1), 0),
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{{
			Index:      0,
			Name:       marker,
			Layout:     "",
			IsActive:   true,
			ActivePane: 0,
			Panes:      panes,
		}},
	}
}

func sampleSnapshot(name string, captured time.Time) snapshot.SessionSnapshot {
	return snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CapturedAt:  captured,
		CurrentWin:  1,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index:      1,
				Name:       "editor",
				IsActive:   true,
				ActivePane: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/tmp", CurrentCmd: "nvim", IsActive: true},
					{Index: 1, CurrentPath: "/tmp", CurrentCmd: "zsh"},
				},
			},
			{
				Index: 2,
				Name:  "shell",
				Panes: []snapshot.Pane{{Index: 0, CurrentCmd: "zsh"}},
			},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := New(dir)

	captured := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in := sampleSnapshot("alpha", captured)

	if err := s.SaveSession(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "sessions", "alpha.json")); err != nil {
		t.Fatalf("session file not written: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		t.Fatalf("index file not written: %v", err)
	}

	out, err := s.LoadSession("alpha")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if out.SessionName != "alpha" || len(out.Windows) != 2 {
		t.Fatalf("unexpected loaded snapshot: %+v", out)
	}

	if !out.CapturedAt.Equal(captured) {
		t.Fatalf("captured time mismatch: got %v want %v", out.CapturedAt, captured)
	}

	if len(out.Windows[0].Panes) != 2 {
		t.Fatalf("expected 2 panes in first window, got %d", len(out.Windows[0].Panes))
	}
}

func TestSaveSessionEmptyNameRejected(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	err := s.SaveSession(snapshot.SessionSnapshot{})
	if err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestSaveSessionDefaultsCapturedAt(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	err := s.SaveSession(snapshot.SessionSnapshot{
		SessionName: "x",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := s.LoadSession("x")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if out.CapturedAt.IsZero() {
		t.Fatal("expected CapturedAt to be defaulted")
	}
}

func TestListRecordsOrderingAndCounts(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	if err := s.SaveSession(sampleSnapshot("beta", older)); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	if err := s.SaveSession(sampleSnapshot("alpha", newer)); err != nil {
		t.Fatalf("save alpha: %v", err)
	}

	recs, err := s.ListRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}

	if recs[0].SessionName != "alpha" {
		t.Fatalf("expected alpha first (newer), got %s", recs[0].SessionName)
	}

	if recs[0].Windows != 2 || recs[0].Panes != 3 {
		t.Fatalf("unexpected counts for alpha: %dw/%dp", recs[0].Windows, recs[0].Panes)
	}
}

func TestListRecordsEmptyStore(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	recs, err := s.ListRecords()
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(recs) != 0 {
		t.Fatalf("expected empty records, got %d", len(recs))
	}
}

func TestLatestRecord(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	if _, err := s.LatestRecord(); !os.IsNotExist(err) {
		t.Fatalf("expected os.ErrNotExist on empty store, got %v", err)
	}

	_ = s.SaveSession(sampleSnapshot("one", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	_ = s.SaveSession(sampleSnapshot("two", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)))

	rec, err := s.LatestRecord()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}

	if rec.SessionName != "two" {
		t.Fatalf("expected latest=two, got %s", rec.SessionName)
	}
}

func TestDeleteSession(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	_ = s.SaveSession(sampleSnapshot("gone", time.Now().UTC()))

	exists, err := s.SessionExists("gone")
	if err != nil || !exists {
		t.Fatalf("expected session to exist, exists=%v err=%v", exists, err)
	}

	if err := s.DeleteSession("gone"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	exists, err = s.SessionExists("gone")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}

	if exists {
		t.Fatal("expected session to be gone after delete")
	}

	in, err := s.IndexEntryExists("gone")
	if err != nil {
		t.Fatalf("index entry exists: %v", err)
	}

	if in {
		t.Fatal("expected index entry removed after delete")
	}
}

func TestDeleteMissingSessionIsNoError(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	err := s.DeleteSession("nope")
	if err != nil {
		t.Fatalf("delete missing should not error: %v", err)
	}
}

func TestMarkSessionAccessed(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())

	_ = s.SaveSession(sampleSnapshot("acc", time.Now().UTC()))

	when := time.Date(2026, 5, 5, 5, 5, 5, 0, time.UTC)
	if err := s.MarkSessionAccessed("acc", when); err != nil {
		t.Fatalf("mark accessed: %v", err)
	}

	recs, _ := s.ListRecords()
	if len(recs) != 1 || !recs[0].LastAccessed.Equal(when) {
		t.Fatalf("expected LastAccessed=%v, got %+v", when, recs)
	}

	if err := s.MarkSessionAccessed("missing", when); !os.IsNotExist(err) {
		t.Fatalf("expected os.ErrNotExist for missing session, got %v", err)
	}
}

func TestSessionNameSanitized(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := New(dir)

	err := s.SaveSession(sampleSnapshot("my/weird name:1", time.Now().UTC()))
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	want := filepath.Join(dir, "sessions", "my_weird_name_1.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected sanitized file %s: %v", want, err)
	}

	if _, err := s.LoadSession("my/weird name:1"); err != nil {
		t.Fatalf("load sanitized: %v", err)
	}
}

func TestScrollbackPersistAndHydrate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := New(dir)

	snap := sampleSnapshot("logs", time.Now().UTC())
	snap.Windows[0].Panes[0].Scrollback = &snapshot.ScrollbackRef{Content: "line1\nline2\n"}

	if err := s.SaveSession(snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	logPath := filepath.Join(dir, "scrollback", "logs", "w1_p0.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("scrollback file not written: %v", err)
	}

	out, err := s.LoadSession("logs")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	sb := out.Windows[0].Panes[0].Scrollback
	if sb == nil || sb.Content != "line1\nline2\n" {
		t.Fatalf("scrollback not hydrated: %+v", sb)
	}

	if sb.Lines != 3 {
		t.Fatalf("expected 3 scrollback lines, got %d", sb.Lines)
	}

	metadata, err := s.LoadSessionMetadata("logs")
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	metadataScrollback := metadata.Windows[0].Panes[0].Scrollback
	if metadataScrollback == nil || metadataScrollback.Ref == "" ||
		metadataScrollback.Content != "" {
		t.Fatalf("metadata load hydrated scrollback content: %+v", metadataScrollback)
	}
}

func TestSessionPath(t *testing.T) {
	t.Parallel()

	s := New("/base")

	p, err := s.SessionPath("name")
	if err != nil {
		t.Fatalf("session path: %v", err)
	}

	if p != filepath.Join("/base", "sessions", "name.json") {
		t.Fatalf("unexpected session path: %s", p)
	}

	if _, err := s.SessionPath("   "); err == nil {
		t.Fatal("expected error for blank session name")
	}
}

func TestDefaultDataDirEnvOverride(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/custom/dir")

	if got := DefaultDataDir(); got != "/custom/dir" {
		t.Fatalf("expected env override, got %s", got)
	}
}
