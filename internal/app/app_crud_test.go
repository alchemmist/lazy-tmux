package app

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func newTestStore(dir string) *store.Store {
	return store.New(dir)
}

func TestNextWindowIndex(t *testing.T) {
	tests := []struct {
		windows []snapshot.Window
		want    int
	}{
		{
			windows: []snapshot.Window{
				{Index: 0},
				{Index: 1},
				{Index: 2},
			},
			want: 3,
		},
		{
			windows: []snapshot.Window{
				{Index: 0},
				{Index: 5},
				{Index: 2},
			},
			want: 6,
		},
		{
			windows: []snapshot.Window{},
			want:    0,
		},
		{
			windows: []snapshot.Window{
				{Index: 10},
			},
			want: 11,
		},
	}

	for _, caseItem := range tests {
		got := nextWindowIndex(caseItem.windows)
		if got != caseItem.want {
			t.Fatalf("nextWindowIndex(%v) = %d, want %d", caseItem.windows, got, caseItem.want)
		}
	}
}

func TestIsShellCommandName(t *testing.T) {
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

func TestDeleteWindowFromStore(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "test-session",
		Windows: []snapshot.Window{
			{Index: 0, Name: "win0", Panes: []snapshot.Pane{{Index: 0}}},
			{Index: 1, Name: "win1", Panes: []snapshot.Pane{{Index: 0}}},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.DeleteWindow("test-session", 1)
	if err != nil {
		t.Fatalf("DeleteWindow: %v", err)
	}

	snap, loadErr := testStore.LoadSession("test-session")
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}

	if len(snap.Windows) != 1 || snap.Windows[0].Index != 0 {
		t.Fatalf("expected 1 window (index 0), got %+v", snap.Windows)
	}
}

func TestDeleteWindowDeletesAllWindows(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "single-win",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.DeleteWindow("single-win", 0)
	if err != nil {
		t.Fatalf("DeleteWindow: %v", err)
	}

	recs, listErr := testStore.ListRecords()
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}

	if len(recs) != 0 {
		t.Fatalf("expected session to be deleted, got %d records", len(recs))
	}
}

func TestDeleteWindowNotFound(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	testStore := newTestStore(dir)

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err := testApp.DeleteWindow("nonexistent", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestDeleteWindowNotFoundInSnapshot(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "test",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.DeleteWindow("test", 99)
	if err == nil {
		t.Fatal("expected error for window not found")
	}
}

func TestDeleteSession(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "to-delete",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.DeleteSession("to-delete")
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	recs, listErr := testStore.ListRecords()
	if listErr != nil {
		t.Fatalf("list: %v", listErr)
	}

	if len(recs) != 0 {
		t.Fatalf("expected session deleted, got %d records", len(recs))
	}
}

func TestRenameWindow(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "rename-test",
		Windows: []snapshot.Window{
			{Index: 0, Name: "old-name", Panes: []snapshot.Pane{{Index: 0}}},
		},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.RenameWindow("rename-test", 0, "new-name")
	if err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}

	snap, loadErr := testStore.LoadSession("rename-test")
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}

	if snap.Windows[0].Name != "new-name" {
		t.Fatalf("expected window name 'new-name', got %q", snap.Windows[0].Name)
	}
}

func TestRenameWindowEmptyName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.RenameWindow("test", 0, "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRenameSession(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "old-session",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.RenameSession("old-session", "new-session")
	if err != nil {
		t.Fatalf("RenameSession: %v", err)
	}

	_, loadErr := testStore.LoadSession("new-session")
	if loadErr != nil {
		t.Fatalf("new session not found: %v", loadErr)
	}

	_, loadErr = testStore.LoadSession("old-session")
	if loadErr == nil {
		t.Fatal("old session should be deleted")
	}
}

func TestRenameSessionEmptyName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.RenameSession("test", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRenameSessionEmptySource(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.RenameSession("", "new")
	if err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestRenameSessionSameName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.RenameSession("test", "test")
	if err != nil {
		t.Fatalf("expected no error for same name, got: %v", err)
	}
}

func TestRenameSessionAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "src",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save src: %v", err)
	}

	err = testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "dst",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save dst: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.RenameSession("src", "dst")
	if err == nil {
		t.Fatal("expected error for existing session")
	}
}

func TestNewSession(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	tmuxClient := &fakeTmuxClient{
		captureSnap: snapshot.SessionSnapshot{
			Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		},
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err := testApp.NewSession("brand-new")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	_, loadErr := testStore.LoadSession("brand-new")
	if loadErr != nil {
		t.Fatalf("session not stored: %v", loadErr)
	}
}

func TestNewSessionEmptyName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.NewSession("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestNewSessionAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "existing",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.NewSession("existing")
	if err == nil {
		t.Fatal("expected error for existing session")
	}
}

func TestNewWindow(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "win-test",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.NewWindow("win-test", "added-window")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	snap, loadErr := testStore.LoadSession("win-test")
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}

	if len(snap.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(snap.Windows))
	}

	if snap.Windows[1].Name != "added-window" {
		t.Fatalf("expected window name 'added-window', got %q", snap.Windows[1].Name)
	}
}

func TestNewWindowEmptySessionName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.NewWindow("", "name")
	if err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestNewWindowGeneratesName(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		SessionName: "gen-test",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.NewWindow("gen-test", "")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	snap, loadErr := testStore.LoadSession("gen-test")
	if loadErr != nil {
		t.Fatalf("load: %v", loadErr)
	}

	if snap.Windows[1].Name != "window-1" {
		t.Fatalf("expected generated name 'window-1', got %q", snap.Windows[1].Name)
	}
}

func TestWakeupRequiresSession(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.Wakeup("")
	if err == nil {
		t.Fatal("expected error for empty session")
	}
}

func TestWakeupAlreadyAwake(t *testing.T) {
	tmuxClient := &fakeTmuxClient{
		sessionExists: true,
	}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.Wakeup("running-session")
	if err == nil {
		t.Fatal("expected error for already awake session")
	}
}

func TestSleepRequiresSession(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.Sleep("")
	if err == nil {
		t.Fatal("expected error for empty session")
	}
}

func TestSleepNotRunning(t *testing.T) {
	tmuxClient := &fakeTmuxClient{
		sessionExists: false,
	}
	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.Sleep("non-running")
	if err == nil {
		t.Fatal("expected error for non-running session")
	}
}

func TestForgetEmptyName(t *testing.T) {
	dir := t.TempDir()
	testCfg := config.Config{
		TmuxBin: "tmux",
		DataDir: dir,
	}
	testApp := NewWithTmux(testCfg, &fakeTmuxClient{})

	err := testApp.Forget("")
	if err == nil {
		t.Fatal("expected error for empty session")
	}
}

func TestForgetNonexistent(t *testing.T) {
	dir := t.TempDir()
	testCfg := config.Config{
		TmuxBin: "tmux",
		DataDir: dir,
	}
	testApp := NewWithTmux(testCfg, &fakeTmuxClient{})

	err := testApp.Forget("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for nonexistent session: %v", err)
	}
}

func TestForgetDeletesStoredSession(t *testing.T) {
	dir := t.TempDir()
	testCfg := config.Config{
		TmuxBin: "tmux",
		DataDir: dir,
	}
	testApp := NewWithTmux(testCfg, &fakeTmuxClient{})

	testStore := newTestStore(dir)

	err := testStore.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "my-session",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("save session: %v", err)
	}

	err = testApp.Forget("my-session")
	if err != nil {
		t.Fatalf("forget session: %v", err)
	}

	exists, err := testStore.SessionExists("my-session")
	if err != nil {
		t.Fatalf("session exists: %v", err)
	}

	if exists {
		t.Fatal("expected session file to be deleted, but it still exists")
	}

	scrollbackExists, err := testStore.ScrollbackExists("my-session")
	if err != nil {
		t.Fatalf("scrollback exists: %v", err)
	}

	if scrollbackExists {
		t.Fatal("expected scrollback data to be deleted, but it still exists")
	}

	indexExists, err := testStore.IndexEntryExists("my-session")
	if err != nil {
		t.Fatalf("index entry exists: %v", err)
	}

	if indexExists {
		t.Fatal("expected index entry to be deleted, but it still exists")
	}
}
