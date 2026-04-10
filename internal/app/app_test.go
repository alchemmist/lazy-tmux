package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

type fakeTmuxClient struct {
	sessions        []string
	currentSession  string
	captureSnap     snapshot.SessionSnapshot
	captureErr      error
	restoreErr      error
	switchErr       error
	paneScrollback  string
	paneScrollErr   error
	listSessionsErr error
	currentSessErr  error
	sessionExists   bool
}

func (f *fakeTmuxClient) ListSessions() ([]string, error) {
	return f.sessions, f.listSessionsErr
}

func (f *fakeTmuxClient) CurrentSession() (string, error) {
	return f.currentSession, f.currentSessErr
}

func (f *fakeTmuxClient) CaptureSession(name string) (snapshot.SessionSnapshot, error) {
	if f.captureErr != nil {
		return snapshot.SessionSnapshot{}, f.captureErr
	}

	snap := f.captureSnap
	snap.SessionName = name

	return snap, nil
}

func (f *fakeTmuxClient) RestoreSession(snap snapshot.SessionSnapshot) error {
	return f.restoreErr
}

func (f *fakeTmuxClient) SwitchClient(target string) error {
	return f.switchErr
}

func (f *fakeTmuxClient) CapturePaneScrollback(target string, lines int) (string, error) {
	return f.paneScrollback, f.paneScrollErr
}

func (f *fakeTmuxClient) NewSession(name string) error                     { return nil }
func (f *fakeTmuxClient) NewWindow(session, name string) error             { return nil }
func (f *fakeTmuxClient) KillWindow(session string, windowIndex int) error { return nil }
func (f *fakeTmuxClient) KillSession(session string) error                 { return nil }
func (f *fakeTmuxClient) RenameWindow(session string, windowIndex int, name string) error {
	return nil
}
func (f *fakeTmuxClient) RenameSession(session, name string) error { return nil }
func (f *fakeTmuxClient) SessionExists(name string) bool {
	return f.sessionExists
}
func (f *fakeTmuxClient) SocketPath() string { return "/tmp/tmux-1000/default" }

func testConfig() config.Config {
	return config.Config{
		TmuxBin:    "tmux",
		DataDir:    "",
		Scrollback: config.ScrollbackConfig{Enabled: false, Lines: 5000},
	}
}

func TestSaveAllIteratesSessions(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{
		sessions: []string{"alpha", "beta"},
		captureSnap: snapshot.SessionSnapshot{
			Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		},
	}

	testApp := NewWithTmux(config.Config{
		TmuxBin:    "tmux",
		DataDir:    dir,
		Scrollback: config.ScrollbackConfig{Enabled: false, Lines: 5000},
	}, tmuxClient)

	err := testApp.SaveAll()
	if err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
}

func TestSaveAllPropagatesListError(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{
		listSessionsErr: fmt.Errorf("no server"),
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.SaveAll()
	if err == nil {
		t.Fatal("expected error from SaveAll")
	}

	if !strings.Contains(err.Error(), "list sessions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveSessionStoresSnapshot(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{
		captureSnap: snapshot.SessionSnapshot{
			Windows: []snapshot.Window{
				{Index: 0, Name: "editor", Panes: []snapshot.Pane{{Index: 0}}},
			},
		},
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.SaveSession("my-session")
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}

	if recs[0].SessionName != "my-session" {
		t.Fatalf("expected session name my-session, got %q", recs[0].SessionName)
	}
}

func TestSaveSessionPropagatesCaptureError(t *testing.T) {
	tmuxClient := &fakeTmuxClient{
		captureErr: fmt.Errorf("capture failed"),
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.SaveSession("test")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "capture session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveCurrentGetsCurrentSession(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{
		currentSession: "current-sess",
		captureSnap: snapshot.SessionSnapshot{
			Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		},
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.SaveCurrent()
	if err != nil {
		t.Fatalf("SaveCurrent: %v", err)
	}

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 1 || recs[0].SessionName != "current-sess" {
		t.Fatalf("expected current-sess, got %+v", recs)
	}
}

func TestSaveCurrentPropagatesError(t *testing.T) {
	tmuxClient := &fakeTmuxClient{
		currentSessErr: fmt.Errorf("no current"),
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)

	err := testApp.SaveCurrent()
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "get current session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestoreTargetLoadsAndRestores(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "restore-me",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.RestoreTarget(PickerTarget{SessionName: "restore-me"}, true)
	if err != nil {
		t.Fatalf("RestoreTarget: %v", err)
	}
}

func TestRestoreTargetRejectsEmptyName(t *testing.T) {
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)

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
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.RestoreTarget(PickerTarget{SessionName: "nonexistent"}, true)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}

	if !strings.Contains(err.Error(), "load session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestoreTargetWithWindowIndex(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "win-target",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	winIdx := 2
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.RestoreTarget(
		PickerTarget{SessionName: "win-target", WindowIndex: &winIdx},
		true,
	)
	if err != nil {
		t.Fatalf("RestoreTarget with window index: %v", err)
	}
}

func TestBootstrapLastWithNoRecords(t *testing.T) {
	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.Bootstrap("last")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
}

func TestBootstrapLastWithRecords(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "bootstrap-sess",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.Bootstrap("last")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
}

func TestBootstrapSpecificSession(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "specific-sess",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	err = testApp.Bootstrap("specific-sess")
	if err != nil {
		t.Fatalf("Bootstrap specific: %v", err)
	}
}

func TestListRecordsDelegatesToStore(t *testing.T) {
	dir := t.TempDir()
	testStore := newTestStore(dir)

	snap := snapshot.SessionSnapshot{
		SessionName: "list-test",
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}

	err := testStore.SaveSession(snap)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	tmuxClient := &fakeTmuxClient{}
	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = testStore

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestCaptureShellScrollbackAddsContentForShellPanes(t *testing.T) {
	snap := &snapshot.SessionSnapshot{
		SessionName: "scroll-session",
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "bash"},
				},
			},
		},
	}

	tmuxClient := &fakeTmuxClient{
		paneScrollback: "echo hello\nhello\n",
	}

	testApp := NewWithTmux(config.Config{
		TmuxBin: "tmux",
		DataDir: "",
		Scrollback: config.ScrollbackConfig{
			Enabled: true,
			Lines:   100,
		},
	}, tmuxClient)

	testApp.captureShellScrollback(snap)

	if snap.Windows[0].Panes[0].Scrollback == nil {
		t.Fatal("expected scrollback to be added")
	}

	if snap.Windows[0].Panes[0].Scrollback.Content != "echo hello\nhello\n" {
		t.Fatalf("unexpected scrollback content: %q", snap.Windows[0].Panes[0].Scrollback.Content)
	}
}

func TestCaptureShellScrollbackSkipsNonShell(t *testing.T) {
	snap := &snapshot.SessionSnapshot{
		SessionName: "noscroll",
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "nvim"},
				},
			},
		},
	}

	tmuxClient := &fakeTmuxClient{
		paneScrollback: "some output",
	}

	testApp := NewWithTmux(config.Config{
		TmuxBin: "tmux",
		DataDir: "",
		Scrollback: config.ScrollbackConfig{
			Enabled: true,
			Lines:   100,
		},
	}, tmuxClient)

	testApp.captureShellScrollback(snap)

	if snap.Windows[0].Panes[0].Scrollback != nil {
		t.Fatal("expected no scrollback for non-shell")
	}
}

func TestCaptureShellScrollbackSkipsEmptyOutput(t *testing.T) {
	snap := &snapshot.SessionSnapshot{
		SessionName: "empty-scroll",
		Windows: []snapshot.Window{
			{
				Index: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentCmd: "zsh"},
				},
			},
		},
	}

	tmuxClient := &fakeTmuxClient{
		paneScrollback: "",
	}

	testApp := NewWithTmux(config.Config{
		TmuxBin: "tmux",
		DataDir: "",
		Scrollback: config.ScrollbackConfig{
			Enabled: true,
			Lines:   100,
		},
	}, tmuxClient)

	testApp.captureShellScrollback(snap)

	if snap.Windows[0].Panes[0].Scrollback != nil {
		t.Fatal("expected no scrollback for empty output")
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

func TestRunDaemonSaveAllFallsBackToSaveAll(t *testing.T) {
	testutil.SkipIfNotIntegration(t)

	dir := t.TempDir()
	tmuxClient := &fakeTmuxClient{
		sessions: []string{"sess1"},
		captureSnap: snapshot.SessionSnapshot{
			Windows: []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		},
	}

	testApp := NewWithTmux(testConfig(), tmuxClient)
	testApp.store = newTestStore(dir)

	err := testApp.runDaemonSaveAll()
	if err != nil {
		t.Fatalf("runDaemonSaveAll: %v", err)
	}

	recs, err := testApp.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}

	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}
