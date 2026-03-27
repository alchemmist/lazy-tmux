package picker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestChooseSessionFZFSuccess(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nprintf 'beta\t2026-02-28 10:00:00\t2w\n'\n")+":"+os.Getenv("PATH"))

	records := []snapshot.Record{
		{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1},
		{SessionName: "beta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3},
	}

	selected, err := ChooseSessionFZF(records)
	if err != nil {
		t.Fatalf("ChooseSessionFZF: %v", err)
	}

	if selected != "beta" {
		t.Fatalf("expected beta, got %q", selected)
	}
}

func TestChooseSessionFZFEmptySelection(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nexit 0\n")+":"+os.Getenv("PATH"))

	_, err := ChooseSessionFZF([]snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}})
	if err == nil {
		t.Fatal("expected error for empty selection")
	}

	if !strings.Contains(err.Error(), "no session selected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChooseSessionFZFCommandFailure(t *testing.T) {
	t.Setenv("PATH", withFakeFZF(t, "#!/bin/sh\nexit 130\n")+":"+os.Getenv("PATH"))

	_, err := ChooseSessionFZF([]snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}})
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
