package app

import (
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/store"
)

func TestSelectWithFZFNoRecords(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}
	_, err := a.SelectWithFZF()
	if err == nil {
		t.Fatal("expected error when there are no records")
	}
	if !strings.Contains(err.Error(), "no saved sessions found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapLastWithoutRecordsReturnsNil(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}
	if err := a.Bootstrap("last"); err != nil {
		t.Fatalf("expected nil when no records exist, got %v", err)
	}
}

func TestAcquireLockIsExclusive(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	unlock1, err := acquireLock("/tmp/tmux.sock")
	if err != nil {
		t.Fatalf("first lock should succeed, got %v", err)
	}
	defer unlock1()

	_, err = acquireLock("/tmp/tmux.sock")
	if err == nil {
		t.Fatal("second lock should fail")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected lock error: %v", err)
	}

	unlock1()

	unlock2, err := acquireLock("/tmp/tmux.sock")
	if err != nil {
		t.Fatalf("lock after unlock should succeed, got %v", err)
	}
	if unlock2 == nil {
		t.Fatal("unlock function must not be nil")
	}
	unlock2()
}

func TestBootstrapEmptyAliasWithoutRecordsReturnsNil(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}
	if err := a.Bootstrap("  "); err != nil {
		t.Fatalf("expected nil for empty alias when no records exist, got %v", err)
	}
}
