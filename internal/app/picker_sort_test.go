package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestParsePickerSortOptions(t *testing.T) {
	opts, err := ParsePickerSortOptions("name:asc,last-used:desc", "name:desc,index:asc")
	if err != nil {
		t.Fatalf("ParsePickerSortOptions error: %v", err)
	}

	gotSession := fmt.Sprint(opts.Session)
	wantSession := fmt.Sprint([]SessionSortKey{
		{Field: SessionSortName, Desc: false},
		{Field: SessionSortLastUsed, Desc: true},
	})
	if gotSession != wantSession {
		t.Fatalf("unexpected session sort keys: got %s want %s", gotSession, wantSession)
	}

	gotWindow := fmt.Sprint(opts.Window)
	wantWindow := fmt.Sprint([]WindowSortKey{
		{Field: WindowSortName, Desc: true},
		{Field: WindowSortIndex, Desc: false},
	})
	if gotWindow != wantWindow {
		t.Fatalf("unexpected window sort keys: got %s want %s", gotWindow, wantWindow)
	}
}

func TestParsePickerSortOptionsRejectsDuplicateField(t *testing.T) {
	_, err := ParsePickerSortOptions("name,name:desc", "")
	if err == nil {
		t.Fatal("expected duplicate sort field error")
	}
}

func TestSortSessionRecordsByCustomPriority(t *testing.T) {
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	records := []snapshot.Record{
		{SessionName: "b", LastAccessed: base.Add(-1 * time.Hour), CapturedAt: base, Windows: 2, Panes: 3},
		{SessionName: "a", LastAccessed: base.Add(-1 * time.Hour), CapturedAt: base.Add(-2 * time.Hour), Windows: 2, Panes: 4},
		{SessionName: "c", LastAccessed: base.Add(-3 * time.Hour), CapturedAt: base, Windows: 5, Panes: 7},
	}

	sortSessionRecords(records, []SessionSortKey{
		{Field: SessionSortWindows, Desc: true},
		{Field: SessionSortName, Desc: false},
	})

	got := []string{records[0].SessionName, records[1].SessionName, records[2].SessionName}
	want := []string{"c", "a", "b"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected session order: got %v want %v", got, want)
	}
}

func TestSortWindowsByCustomPriority(t *testing.T) {
	windows := []snapshot.Window{
		{Index: 2, Name: "logs", Panes: []snapshot.Pane{{Index: 0}}},
		{Index: 0, Name: "editor", Panes: []snapshot.Pane{{Index: 0}, {Index: 1}}},
		{Index: 1, Name: "shell", Panes: []snapshot.Pane{{Index: 0}, {Index: 1}, {Index: 2}}},
	}

	sortWindows(windows, []WindowSortKey{
		{Field: WindowSortPanes, Desc: true},
		{Field: WindowSortName, Desc: false},
	})

	got := []string{windows[0].Name, windows[1].Name, windows[2].Name}
	want := []string{"shell", "editor", "logs"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected window order: got %v want %v", got, want)
	}
}
