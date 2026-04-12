package app

import (
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestRestoreTargetRejectsEmptyName(t *testing.T) {
	testApp := New(config.Config{TmuxBin: "tmux"})

	err := testApp.RestoreTarget(PickerTarget{}, true)
	if err == nil {
		t.Fatal("expected error for empty session name")
	}

	if !strings.Contains(err.Error(), "empty session name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestoreTargetPropagatesLoadError(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}
	testApp := NewWithStore(cfg, store.New(dir))

	err := testApp.RestoreTarget(PickerTarget{SessionName: "nonexistent"}, true)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}

	if !strings.Contains(err.Error(), "load session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapLastWithNoRecords(t *testing.T) {
	testutil.SkipIfNotIntegration(t)
	testutil.RequireTMux(t)

	dir := t.TempDir()
	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}
	testApp := NewWithStore(cfg, store.New(dir))

	err := testApp.Bootstrap("last")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
}

func TestBootstrapLastWithRecords(t *testing.T) {
	testutil.SkipIfNotIntegration(t)
	testutil.RequireTMux(t)

	dir := t.TempDir()
	testStore := store.New(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "bootstrap-sess",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}
	testApp := NewWithStore(cfg, testStore)

	err = testApp.Bootstrap("last")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
}

func TestBootstrapSpecificSession(t *testing.T) {
	testutil.SkipIfNotIntegration(t)
	testutil.RequireTMux(t)

	dir := t.TempDir()
	testStore := store.New(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "specific-sess",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}
	testApp := NewWithStore(cfg, testStore)

	err = testApp.Bootstrap("specific-sess")
	if err != nil {
		t.Fatalf("Bootstrap specific: %v", err)
	}
}

func TestListRecordsDelegatesToStore(t *testing.T) {
	dir := t.TempDir()
	testStore := store.New(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "list-test",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}
	testApp := NewWithStore(cfg, testStore)

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestIsShellCommandNameTable(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"bash", true},
		{"-bash", true},
		{"/bin/bash", true},
		{"/bin/bash -l", true},
		{"zsh", true},
		{"fish", true},
		{"sh", true},
		{"ksh", true},
		{"nvim", false},
		{"vim", false},
		{"docker", false},
		{"", false},
		{"   ", false},
	}

	for _, caseItem := range tests {
		if got := isShellCommandName(caseItem.cmd); got != caseItem.want {
			t.Fatalf("isShellCommandName(%q) = %v, want %v", caseItem.cmd, got, caseItem.want)
		}
	}
}

func TestRunDaemonSaveAllUsesSaveAllFn(t *testing.T) {
	called := false
	testApp := &App{
		saveAllFn: func() error {
			called = true
			return nil
		},
	}

	err := testApp.runDaemonSaveAll()
	if err != nil {
		t.Fatalf("runDaemonSaveAll: %v", err)
	}

	if !called {
		t.Fatal("expected saveAllFn to be called")
	}
}
