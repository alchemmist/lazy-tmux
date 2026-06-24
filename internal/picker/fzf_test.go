package picker

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestChooseSessionFZFEmpty(t *testing.T) {
	if _, err := ChooseSessionFZF(nil); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("expected ErrNoSessions, got %v", err)
	}
}

func TestWindowFZFLinesSortedAndFormatted(t *testing.T) {
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

	// First line must be window index 1 (editor/nvim) after sorting.
	first := strings.Split(lines[0], "\t")
	if first[0] != "work" || first[1] != "1" || first[2] != "editor" || first[3] != "nvim" {
		t.Fatalf("unexpected first line fields: %#v", first)
	}

	second := strings.Split(lines[1], "\t")
	if second[1] != "2" || second[2] != "shell" {
		t.Fatalf("unexpected second line fields: %#v", second)
	}
}

func TestParseWindowSelection(t *testing.T) {
	target, err := parseWindowSelection("work\t3\teditor\tnvim\t2026-01-02 03:04:05")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if target.SessionName != "work" || target.WindowIndex == nil || *target.WindowIndex != 3 {
		t.Fatalf("unexpected target: %+v", target)
	}

	if _, err := parseWindowSelection("work"); err == nil {
		t.Fatal("expected error for line without a window index")
	}

	if _, err := parseWindowSelection("\t1"); err == nil {
		t.Fatal("expected error for empty session")
	}

	if _, err := parseWindowSelection("work\tNaN"); err == nil {
		t.Fatal("expected error for non-numeric window index")
	}
}

func TestChooseWindowFZFEmpty(t *testing.T) {
	if _, err := ChooseWindowFZF(nil, nil); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("expected ErrNoSessions, got %v", err)
	}
}

func TestChooseWindowFZFNoWindows(t *testing.T) {
	// Sessions exist but carry no windows: this is distinct from "no sessions".
	sessions := []Session{
		{Record: snapshot.Record{SessionName: "work", CapturedAt: time.Now()}},
	}

	if _, err := ChooseWindowFZF(sessions, nil); !errors.Is(err, ErrNoWindows) {
		t.Fatalf("expected ErrNoWindows, got %v", err)
	}
}

func TestChooseWindowFZFFilterMode(t *testing.T) {
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
