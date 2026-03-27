package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestPickerRecordsEmpty(t *testing.T) {
	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: "tmux"})

	_, err := a.pickerRecords(DefaultPickerSortOptions())
	if err == nil {
		t.Fatal("expected error for empty records")
	}

	if !strings.Contains(err.Error(), "no saved sessions found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPickerRecordsSortedByCapturedAt(t *testing.T) {
	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: "tmux"})
	base := time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC)

	snaps := []snapshot.SessionSnapshot{
		{Version: snapshot.FormatVersion, SessionName: "old", CapturedAt: base.Add(-2 * time.Hour), Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}}},
		{Version: snapshot.FormatVersion, SessionName: "new", CapturedAt: base.Add(-1 * time.Hour), Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}}},
		{Version: snapshot.FormatVersion, SessionName: "latest", CapturedAt: base, Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}}},
	}
	for _, s := range snaps {
		if err := a.store.SaveSession(s); err != nil {
			t.Fatalf("save session %q: %v", s.SessionName, err)
		}
	}

	recs, err := a.pickerRecords(DefaultPickerSortOptions())
	if err != nil {
		t.Fatalf("pickerRecords: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}

	got := []string{recs[0].SessionName, recs[1].SessionName, recs[2].SessionName}
	want := []string{"latest", "new", "old"}

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected order: got %v want %v", got, want)
	}
}

func TestPickerRecordsSortedByLastAccessed(t *testing.T) {
	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: "tmux"})
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	for _, s := range []snapshot.SessionSnapshot{
		{Version: snapshot.FormatVersion, SessionName: "alpha", CapturedAt: base, Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}}},
		{Version: snapshot.FormatVersion, SessionName: "beta", CapturedAt: base.Add(1 * time.Hour), Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}}},
	} {
		if err := a.store.SaveSession(s); err != nil {
			t.Fatalf("save session %q: %v", s.SessionName, err)
		}
	}

	// Access alpha later than beta: alpha should be listed first in picker.
	if err := a.store.MarkSessionAccessed("beta", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("mark beta: %v", err)
	}

	if err := a.store.MarkSessionAccessed("alpha", base.Add(3*time.Hour)); err != nil {
		t.Fatalf("mark alpha: %v", err)
	}

	recs, err := a.pickerRecords(DefaultPickerSortOptions())
	if err != nil {
		t.Fatalf("pickerRecords: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}

	if recs[0].SessionName != "alpha" || recs[1].SessionName != "beta" {
		t.Fatalf("unexpected order by last_accessed: %#v", recs)
	}
}
