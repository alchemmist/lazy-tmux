//go:build !lazy_fzf

package picker

import (
	"fmt"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestFilteredTreeRowsByWindowNameKeepsSessionParent(t *testing.T) {
	base := time.Date(2026, 2, 28, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
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

	rows := filteredTreeRows(sessions, "log", DefaultSortOptions().Window)
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
		"item",
		"cmd",
		"captured",
		"wins",
		"state",
	}

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected columns for wide layout: got %v want %v", got, want)
	}
}

func TestBuildPickerTableLayoutNarrowHidesLowPriorityColumns(t *testing.T) {
	layout := buildPickerTableLayout(32)
	got := columnIDs(layout)
	want := []pickerColumnID{
		"item",
		"cmd",
	}

	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("unexpected columns for narrow layout: got %v want %v", got, want)
	}
}

func TestBuildPickerTableLayoutKeepsRequiredItemColumn(t *testing.T) {
	width := 8
	layout := buildPickerTableLayout(width)
	got := columnIDs(layout)
	want := []pickerColumnID{"item"}

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
