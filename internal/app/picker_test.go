package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestPickerRecordsEmpty(t *testing.T) {
	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: "tmux"})

	_, err := a.pickerRecords()
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

	recs, err := a.pickerRecords()
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

func TestChooseSessionFZFSuccess(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nprintf 'beta\t2026-02-28 10:00:00\t2w\n'\n")+":"+os.Getenv("PATH"))

	records := []snapshot.Record{
		{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1},
		{SessionName: "beta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3},
	}

	selected, err := chooseSessionFZF(records)
	if err != nil {
		t.Fatalf("chooseSessionFZF: %v", err)
	}
	if selected != "beta" {
		t.Fatalf("expected beta, got %q", selected)
	}
}

func TestChooseSessionFZFEmptySelection(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nexit 0\n")+":"+os.Getenv("PATH"))

	_, err := chooseSessionFZF([]snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}})
	if err == nil {
		t.Fatal("expected error for empty selection")
	}
	if !strings.Contains(err.Error(), "no session selected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChooseSessionFZFCommandFailure(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nexit 130\n")+":"+os.Getenv("PATH"))

	_, err := chooseSessionFZF([]snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}})
	if err == nil {
		t.Fatal("expected command failure error")
	}
	if !strings.Contains(err.Error(), "fzf selection canceled or failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func withFakeFZF(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fzf: %v", err)
	}
	return dir
}