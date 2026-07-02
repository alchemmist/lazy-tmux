package picker

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/charmbracelet/x/ansi"
)

func TestChooseSessionFZFEmpty(t *testing.T) {
	t.Parallel()

	if _, err := ChooseSessionFZF(nil); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("expected ErrNoSessions, got %v", err)
	}
}

func TestWindowFZFLinesSortedAndFormatted(t *testing.T) {
	t.Parallel()

	sessions := []Session{
		{
			Record: snapshot.Record{
				SessionName: "work",
				CapturedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			Windows: []snapshot.Window{
				{Index: 2, Name: "shell", Panes: []snapshot.Pane{{Index: 0, CurrentCmd: "zsh"}}},
				{Index: 1, Name: "editor", Panes: []snapshot.Pane{{Index: 0, RestoreCmd: "nvim"}}},
			},
		},
	}

	// Sort windows by index ascending.
	lines := windowFZFLines(sessions, []WindowSortKey{{Field: WindowSortIndex, Desc: false}})

	if len(lines) != 2 {
		t.Fatalf("expected 2 window lines, got %d: %#v", len(lines), lines)
	}

	// First line must be window index 1 (editor/nvim) after sorting; parse the
	// hidden fields, and confirm the display column carries the visible values.
	first, err := parseWindowSelection(lines[0])
	if err != nil {
		t.Fatalf("parse first line: %v", err)
	}

	if first.SessionName != "work" || first.WindowIndex == nil || *first.WindowIndex != 1 {
		t.Fatalf("unexpected first target: %+v", first)
	}

	if display := strings.SplitN(lines[0], "\t", 2)[0]; !strings.Contains(display, "editor") ||
		!strings.Contains(display, "nvim") {
		t.Fatalf("first display column missing name/cmd: %q", display)
	}

	second, err := parseWindowSelection(lines[1])
	if err != nil {
		t.Fatalf("parse second line: %v", err)
	}

	if second.WindowIndex == nil || *second.WindowIndex != 2 {
		t.Fatalf("unexpected second target: %+v", second)
	}

	// The visible column (everything before the first tab) must be the same
	// rendered width on every row, or fzf would show ragged columns (#200).
	assertEqualDisplayWidths(t, lines)
}

func TestSessionFZFLinesAligned(t *testing.T) {
	t.Parallel()

	records := []snapshot.Record{
		{
			SessionName: "tmp",
			CapturedAt:  time.Date(2026, 7, 1, 16, 18, 42, 0, time.UTC),
			Windows:     3,
		},
		{
			SessionName: "arcadia-burn",
			CapturedAt:  time.Date(2026, 6, 30, 18, 11, 15, 0, time.UTC),
			Windows:     2,
		},
		{
			SessionName: "dotfiles",
			CapturedAt:  time.Date(2026, 5, 27, 20, 2, 20, 0, time.UTC),
			Windows:     12,
		},
	}

	lines := sessionFZFLines(records)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Different name lengths must not shift the date column: identical display
	// widths, and the timestamp starts at the same offset on every row.
	assertEqualDisplayWidths(t, lines)

	off := -1

	for _, line := range lines {
		display := strings.SplitN(line, "\t", 2)[0]

		at := strings.Index(display, "202") // start of the year in the timestamp
		if off == -1 {
			off = at
		} else if at != off {
			t.Fatalf("timestamp column misaligned: offset %d != %d in %q", at, off, display)
		}
	}

	// The hidden trailing field must carry the real session name for parsing.
	if got := strings.Split(lines[0], "\t"); got[len(got)-1] != "tmp" {
		t.Fatalf("hidden name field wrong: %#v", got)
	}
}

// assertEqualDisplayWidths checks that the visible column (before the first tab)
// has the same rendered width on every line.
func assertEqualDisplayWidths(t *testing.T, lines []string) {
	t.Helper()

	want := -1

	for _, line := range lines {
		display := strings.SplitN(line, "\t", 2)[0]

		got := ansi.StringWidth(display)
		if want == -1 {
			want = got
		} else if got != want {
			t.Fatalf("display width %d != %d for %q", got, want, display)
		}
	}
}

func TestParseWindowSelection(t *testing.T) {
	t.Parallel()

	target, err := parseWindowSelection("editor  nvim  2026-01-02 03:04:05\twork\t3")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if target.SessionName != "work" || target.WindowIndex == nil || *target.WindowIndex != 3 {
		t.Fatalf("unexpected target: %+v", target)
	}

	if _, err := parseWindowSelection("work"); err == nil {
		t.Fatal("expected error for line without hidden fields")
	}

	if _, err := parseWindowSelection("display\t\t1"); err == nil {
		t.Fatal("expected error for empty session")
	}

	if _, err := parseWindowSelection("display\twork\tNaN"); err == nil {
		t.Fatal("expected error for non-numeric window index")
	}
}

func TestChooseWindowFZFEmpty(t *testing.T) {
	t.Parallel()

	if _, err := ChooseWindowFZF(nil, nil); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("expected ErrNoSessions, got %v", err)
	}
}

func TestChooseWindowFZFNoWindows(t *testing.T) {
	t.Parallel()

	// Sessions exist but carry no windows: this is distinct from "no sessions".
	sessions := []Session{
		{Record: snapshot.Record{SessionName: "work", CapturedAt: time.Now()}},
	}

	if _, err := ChooseWindowFZF(sessions, nil); !errors.Is(err, ErrNoWindows) {
		t.Fatalf("expected ErrNoWindows, got %v", err)
	}
}

func TestChooseWindowFZFFilterMode(t *testing.T) {
	t.Parallel()

	testutil.RequireFZF(t)

	// Non-interactive: fzf runs in --filter mode and the first (sorted) window
	// line is selected, exercising the real fzf binary end-to-end.
	sessions := []Session{
		{
			Record: snapshot.Record{SessionName: "work", CapturedAt: time.Now()},
			Windows: []snapshot.Window{
				{Index: 1, Name: "editor", Panes: []snapshot.Pane{{Index: 0, RestoreCmd: "nvim"}}},
				{Index: 2, Name: "shell", Panes: []snapshot.Pane{{Index: 0, CurrentCmd: "zsh"}}},
			},
		},
	}

	target, err := ChooseWindowFZF(sessions, []WindowSortKey{{Field: WindowSortIndex}})
	if err != nil {
		t.Fatalf("choose window fzf: %v", err)
	}

	if target.SessionName != "work" || target.WindowIndex == nil || *target.WindowIndex != 1 {
		t.Fatalf("expected work window 1, got %+v", target)
	}
}

func TestChooseSessionFZFFilterMode(t *testing.T) {
	t.Parallel()

	testutil.RequireFZF(t)

	// In a non-interactive (no TTY) context, ChooseSessionFZF invokes fzf in
	// --filter mode, which prints all matching lines without any user input.
	// This exercises the real fzf binary end-to-end and returns the first
	// session name.
	records := []snapshot.Record{
		{SessionName: "alpha", CapturedAt: time.Now(), Windows: 2},
		{SessionName: "beta", CapturedAt: time.Now(), Windows: 1},
	}

	got, err := ChooseSessionFZF(records)
	if err != nil {
		t.Fatalf("choose session fzf: %v", err)
	}

	if got != "alpha" {
		t.Fatalf("expected first session 'alpha', got %q", got)
	}
}
