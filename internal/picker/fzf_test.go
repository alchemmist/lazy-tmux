package picker

import (
	"errors"
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
