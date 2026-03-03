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
		{Version: snapshot.FormatVersion, SessionName: "alpha", CapturedAt: base, Windows: []snapshot.Window{{Index: 0}}},
		{Version: snapshot.FormatVersion, SessionName: "beta", CapturedAt: base.Add(1 * time.Hour), Windows: []snapshot.Window{{Index: 0}}},
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

func TestFilteredTreeRowsByWindowNameKeepsSessionParent(t *testing.T) {
	base := time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC)
	sessions := []pickerSession{
		{
			Record: snapshot.Record{
				SessionName: "work",
				CapturedAt:  base,
				Windows:     2,
			},
			Windows: []snapshot.Window{
				{Index: 0, Name: "editor"},
				{Index: 1, Name: "logs"},
			},
		},
	}

	rows := filteredTreeRows(sessions, "log", DefaultPickerSortOptions().Window)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (session + matched window), got %d", len(rows))
	}
	if rows[0].target.SessionName != "work" || rows[0].target.WindowIndex != nil {
		t.Fatalf("unexpected parent row target: %+v", rows[0].target)
	}
	if rows[0].selectable {
		t.Fatalf("session row must be non-selectable")
	}
	if rows[1].target.WindowIndex == nil || *rows[1].target.WindowIndex != 1 {
		t.Fatalf("expected selected window index 1, got %+v", rows[1].target)
	}
	if !rows[1].selectable {
		t.Fatalf("window row must be selectable")
	}
}

func TestMoveSkipsSessionHeaders(t *testing.T) {
	m := pickerModel{
		visible: []pickerRow{
			{item: "session-a", selectable: false},
			{item: "  ├─ [0] editor", selectable: true},
			{item: "  ╰─ [1] logs", selectable: true},
			{item: "session-b", selectable: false},
			{item: "  ╰─ [0] shell", selectable: true},
		},
		cursor: 2,
	}

	m.moveNextSelectable()
	if got := m.cursor; got != 4 {
		t.Fatalf("expected jump to first window of next session, got cursor=%d", got)
	}

	m.movePrevSelectable()
	if got := m.cursor; got != 2 {
		t.Fatalf("expected jump back to previous selectable row, got cursor=%d", got)
	}
}

func TestWindowPreviewCommandUsesActivePaneByIndex(t *testing.T) {
	w := snapshot.Window{
		ActivePane: 5,
		Panes: []snapshot.Pane{
			{Index: 2, CurrentCmd: "bash"},
			{Index: 5, RestoreCmd: "nvim main.go", CurrentCmd: "nvim"},
		},
	}

	got := windowPreviewCommand(w)
	if got != "nvim main.go" {
		t.Fatalf("expected active pane restore command, got %q", got)
	}
}

func TestWindowPreviewCommandFallsBackToFirstPane(t *testing.T) {
	w := snapshot.Window{
		ActivePane: 9,
		Panes: []snapshot.Pane{
			{Index: 1, CurrentCmd: "htop"},
			{Index: 3, CurrentCmd: "bash"},
		},
	}

	got := windowPreviewCommand(w)
	if got != "htop" {
		t.Fatalf("expected fallback to first pane command, got %q", got)
	}
}

func TestBuildPickerTableLayoutWideShowsAllColumns(t *testing.T) {
	layout := buildPickerTableLayout(90)
	got := columnIDs(layout)
	want := []pickerColumnID{
		pickerColItem,
		pickerColCmd,
		pickerColCaptured,
		pickerColWins,
		pickerColState,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected columns for wide layout: got %v want %v", got, want)
	}
}

func TestBuildPickerTableLayoutNarrowHidesLowPriorityColumns(t *testing.T) {
	layout := buildPickerTableLayout(32)
	got := columnIDs(layout)
	want := []pickerColumnID{
		pickerColItem,
		pickerColCmd,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected columns for narrow layout: got %v want %v", got, want)
	}
}

func TestBuildPickerTableLayoutKeepsRequiredItemColumn(t *testing.T) {
	width := 8
	layout := buildPickerTableLayout(width)
	got := columnIDs(layout)
	want := []pickerColumnID{pickerColItem}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("required item column must remain visible: got %v want %v", got, want)
	}
	if gotWidth := len([]rune(layout.header())); gotWidth > width {
		t.Fatalf("layout width exceeds budget: got %d want <= %d", gotWidth, width)
	}
}

func columnIDs(layout pickerTableLayout) []pickerColumnID {
	out := make([]pickerColumnID, 0, len(layout.columns))
	for _, col := range layout.columns {
		out = append(out, col.spec.ID)
	}
	return out
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
