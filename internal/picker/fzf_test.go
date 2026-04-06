//go:build integration && !lazy_fzf

package picker

import (
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestChooseSessionFZFSelectsFirst(t *testing.T) {
	records := []snapshot.Record{
		{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1},
		{SessionName: "beta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3},
	}

	selected, err := ChooseSessionFZF(records)
	if err != nil {
		t.Fatalf("ChooseSessionFZF: %v", err)
	}

	if selected != "alpha" {
		t.Fatalf("expected alpha, got %q", selected)
	}
}

func TestChooseSessionFZFNoSessions(t *testing.T) {
	_, err := ChooseSessionFZF([]snapshot.Record{})
	if err == nil {
		t.Fatal("expected error for empty records")
	}

	if !strings.Contains(err.Error(), "no sessions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChooseSessionFZFOrderPreserved(t *testing.T) {
	records := []snapshot.Record{
		{SessionName: "gamma", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1},
		{SessionName: "delta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3},
		{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 3, Panes: 5},
	}

	selected, err := ChooseSessionFZF(records)
	if err != nil {
		t.Fatalf("ChooseSessionFZF: %v", err)
	}

	if selected != "gamma" {
		t.Fatalf("expected gamma (first in input), got %q", selected)
	}
}
