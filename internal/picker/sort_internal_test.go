package picker

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestDefaultSortOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultSortOptions()

	if len(opts.Session) == 0 || opts.Session[0].Field != SessionSortLastUsed ||
		!opts.Session[0].Desc {
		t.Fatalf("unexpected default session sort: %+v", opts.Session)
	}

	if len(opts.Window) == 0 || opts.Window[0].Field != WindowSortIndex {
		t.Fatalf("unexpected default window sort: %+v", opts.Window)
	}
}

func TestParseSortOptionsValid(t *testing.T) {
	t.Parallel()

	opts, err := ParseSortOptions("name:asc,panes:desc", "cmd,index:desc")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(opts.Session) != 2 || opts.Session[0].Field != SessionSortName || opts.Session[0].Desc {
		t.Fatalf("unexpected session keys: %+v", opts.Session)
	}

	if opts.Session[1].Field != SessionSortPanes || !opts.Session[1].Desc {
		t.Fatalf("unexpected session key[1]: %+v", opts.Session[1])
	}

	if len(opts.Window) != 2 || opts.Window[0].Field != WindowSortCmd {
		t.Fatalf("unexpected window keys: %+v", opts.Window)
	}

	// Window cmd default direction is ascending; index:desc explicit.
	if opts.Window[1].Field != WindowSortIndex || !opts.Window[1].Desc {
		t.Fatalf("unexpected window key[1]: %+v", opts.Window[1])
	}
}

func TestParseSortOptionsEmptyKeepsDefaults(t *testing.T) {
	t.Parallel()

	opts, err := ParseSortOptions("", "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	def := DefaultSortOptions()
	if len(opts.Session) != len(def.Session) || len(opts.Window) != len(def.Window) {
		t.Fatalf("empty exprs should keep defaults: %+v", opts)
	}
}

func TestParseSortOptionsDefaultDirections(t *testing.T) {
	t.Parallel()

	// panes (window) defaults to desc; name defaults to asc.
	opts, err := ParseSortOptions("captured", "panes")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if opts.Session[0].Field != SessionSortCaptured || !opts.Session[0].Desc {
		t.Fatalf("captured should default desc: %+v", opts.Session[0])
	}

	if opts.Window[0].Field != WindowSortPanes || !opts.Window[0].Desc {
		t.Fatalf("window panes should default desc: %+v", opts.Window[0])
	}
}

func TestParseSortOptionsErrors(t *testing.T) {
	t.Parallel()

	cases := []struct{ s, w string }{
		{"bogus", ""},
		{"name:sideways", ""},
		{"name,name", ""},
		{",", ""},
		{"", "bogus"},
		{"", "index,index"},
	}

	for _, c := range cases {
		if _, err := ParseSortOptions(c.s, c.w); err == nil {
			t.Fatalf("expected error for session=%q window=%q", c.s, c.w)
		}
	}
}

func TestSortSessionRecords(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	recs := []snapshot.Record{
		{SessionName: "b", CapturedAt: t1, Windows: 1},
		{SessionName: "a", CapturedAt: t2, Windows: 3},
		{SessionName: "c", CapturedAt: t2, Windows: 2},
	}

	SortSessionRecords(recs, []SessionSortKey{{Field: SessionSortName, Desc: false}})

	if recs[0].SessionName != "a" || recs[2].SessionName != "c" {
		t.Fatalf("name asc failed: %+v", recs)
	}

	SortSessionRecords(recs, []SessionSortKey{{Field: SessionSortWindows, Desc: true}})

	if recs[0].Windows != 3 {
		t.Fatalf("windows desc failed: %+v", recs)
	}

	// captured desc, tie-break by name asc.
	SortSessionRecords(recs, []SessionSortKey{{Field: SessionSortCaptured, Desc: true}})

	if recs[0].SessionName != "a" || recs[1].SessionName != "c" || recs[2].SessionName != "b" {
		t.Fatalf("captured desc + name tiebreak failed: %+v", recs)
	}
}

func TestSortWindows(t *testing.T) {
	t.Parallel()

	wins := []snapshot.Window{
		{Index: 3, Name: "z", Panes: []snapshot.Pane{{}}},
		{Index: 1, Name: "a", Panes: []snapshot.Pane{{}, {}, {}}},
		{Index: 2, Name: "m", Panes: []snapshot.Pane{{}, {}}},
	}

	SortWindows(wins, []WindowSortKey{{Field: WindowSortIndex, Desc: false}})

	if wins[0].Index != 1 || wins[2].Index != 3 {
		t.Fatalf("index asc failed: %+v", wins)
	}

	SortWindows(wins, []WindowSortKey{{Field: WindowSortPanes, Desc: true}})

	if len(wins[0].Panes) != 3 {
		t.Fatalf("panes desc failed: %+v", wins)
	}

	SortWindows(wins, []WindowSortKey{{Field: WindowSortName, Desc: false}})

	if wins[0].Name != "a" || wins[2].Name != "z" {
		t.Fatalf("name asc failed: %+v", wins)
	}
}

func TestWindowPreviewCommand(t *testing.T) {
	t.Parallel()

	// Prefers RestoreCmd of the active pane; falls back to CurrentCmd.
	win := snapshot.Window{
		ActivePane: 1,
		Panes: []snapshot.Pane{
			{Index: 0, CurrentCmd: "zsh"},
			{Index: 1, CurrentCmd: "nvim", RestoreCmd: "nvim ."},
		},
	}

	if got := windowPreviewCommand(win); got != "nvim ." {
		t.Fatalf("expected restore cmd, got %q", got)
	}

	win.Panes[1].RestoreCmd = ""
	if got := windowPreviewCommand(win); got != "nvim" {
		t.Fatalf("expected current cmd fallback, got %q", got)
	}

	if got := windowPreviewCommand(snapshot.Window{}); got != "" {
		t.Fatalf("expected empty for no panes, got %q", got)
	}
}
