//go:build !lazy_fzf

package picker

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestFilteredTreeRowsSessionMatchIncludesAllWindows(t *testing.T) {
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{
			Record: snapshot.Record{
				SessionName: "work",
				CapturedAt:  base,
				Windows:     2,
			},
			Windows: []snapshot.Window{
				{Index: 2, Name: "logs"},
				{Index: 1, Name: "editor"},
			},
		},
	}

	rows := filteredTreeRows(sessions, "wor", DefaultSortOptions().Window)
	if len(rows) != 3 {
		t.Fatalf("expected session row + 2 window rows, got %d", len(rows))
	}
	if rows[1].windowName != "editor" {
		t.Fatalf("expected windows to be sorted by index, got first window %q", rows[1].windowName)
	}
	if rows[2].windowName != "logs" {
		t.Fatalf("unexpected second window: %q", rows[2].windowName)
	}
}
