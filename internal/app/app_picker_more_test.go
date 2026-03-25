package app

import (
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

func TestSelectWithFZFSortedNoRecords(t *testing.T) {
	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: "tmux"})
	_, err := a.SelectWithFZFSorted(DefaultPickerSortOptions())
	if err == nil {
		t.Fatal("expected error for empty records")
	}
	if !strings.Contains(err.Error(), "no saved sessions found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
