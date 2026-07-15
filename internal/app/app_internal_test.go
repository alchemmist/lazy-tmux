package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

type recordingTmux struct {
	tmuxClient

	inside   bool
	switched string
	attached string
}

func (r *recordingTmux) RestoreSession(context.Context, snapshot.SessionSnapshot) error {
	return nil
}
func (r *recordingTmux) InsideTmux() bool { return r.inside }

func (r *recordingTmux) SwitchClient(
	target string,
) error {
	r.switched = target

	return nil
}

func (r *recordingTmux) AttachSession(
	target string,
) error {
	r.attached = target

	return nil
}

func TestRestoreTargetHandsOff(t *testing.T) {
	t.Parallel()

	idx := 2

	cases := []struct {
		name         string
		interactive  bool
		inside       bool
		windowIndex  *int
		wantSwitched string
		wantAttached string
	}{
		{name: "cli inside tmux switches", inside: true, wantSwitched: "s1"},
		{name: "picker inside tmux switches", interactive: true, inside: true, wantSwitched: "s1"},

		{name: "cli outside tmux does not attach", inside: false},
		{
			name:         "picker outside tmux attaches",
			interactive:  true,
			inside:       false,
			wantAttached: "s1",
		},
		{
			name:         "picker outside tmux attaches to window",
			interactive:  true,
			inside:       false,
			windowIndex:  &idx,
			wantAttached: "s1:2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, _ := newTestApp(t)

			if err := a.store.SaveSession(snapshot.SessionSnapshot{SessionName: "s1"}); err != nil {
				t.Fatalf("save snapshot: %v", err)
			}

			fake := &recordingTmux{inside: tc.inside}
			a.tmux = fake

			target := PickerTarget{SessionName: "s1", WindowIndex: tc.windowIndex}

			var err error
			if tc.interactive {
				err = a.RestoreTargetInteractive(target)
			} else {
				err = a.RestoreTarget(target, true)
			}

			if err != nil {
				t.Fatalf("restore target: %v", err)
			}

			if fake.switched != tc.wantSwitched {
				t.Fatalf("switched = %q, want %q", fake.switched, tc.wantSwitched)
			}

			if fake.attached != tc.wantAttached {
				t.Fatalf("attached = %q, want %q", fake.attached, tc.wantAttached)
			}
		})
	}
}

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()

	dir := t.TempDir()
	cfg := config.Config{TmuxBin: "tmux", DataDir: dir}

	return New(cfg), dir
}

func snapshotExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, "sessions", name+".json"))

	return err == nil
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestNewSessionCreatesAndSaves(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, dir := newTestApp(t)

	if err := a.NewSession("fresh"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	if !a.tmux.SessionExists("fresh") {
		t.Fatal("expected tmux session to exist")
	}

	if !snapshotExists(dir, "fresh") {
		t.Fatal("expected snapshot to be saved")
	}

	if err := a.NewSession("fresh"); err == nil {
		t.Fatal("expected error creating duplicate session")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestNewWindowLiveAndOffline(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)

	if err := a.NewSession("multi"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := a.NewWindow("multi", "second"); err != nil {
		t.Fatalf("new window (live): %v", err)
	}

	snap, err := a.store.LoadSession("multi")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(snap.Windows) != 2 {
		t.Fatalf("expected 2 windows after live new-window, got %d", len(snap.Windows))
	}

	if err := a.tmux.KillSession("multi"); err != nil {
		t.Fatalf("kill: %v", err)
	}

	if err := a.NewWindow("multi", "third"); err != nil {
		t.Fatalf("new window (offline): %v", err)
	}

	snap, err = a.store.LoadSession("multi")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if len(snap.Windows) != 3 {
		t.Fatalf("expected 3 windows after offline new-window, got %d", len(snap.Windows))
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRenameWindowLive(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)

	if err := a.NewSession("ren"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := a.RenameWindow("ren", 0, "renamed"); err != nil {
		t.Fatalf("rename window: %v", err)
	}

	snap, err := a.store.LoadSession("ren")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if snap.Windows[0].Name != "renamed" {
		t.Fatalf("expected window renamed in snapshot, got %q", snap.Windows[0].Name)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRenameSession(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, dir := newTestApp(t)

	if err := a.NewSession("old"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := a.RenameSession("old", "brandnew"); err != nil {
		t.Fatalf("rename session: %v", err)
	}

	if a.tmux.SessionExists("old") {
		t.Fatal("old session should be gone in tmux")
	}

	if !a.tmux.SessionExists("brandnew") {
		t.Fatal("new session should exist in tmux")
	}

	if snapshotExists(dir, "old") || !snapshotExists(dir, "brandnew") {
		t.Fatal("snapshot file should be moved old -> brandnew")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestDeleteWindowAndSession(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, dir := newTestApp(t)

	if err := a.NewSession("del"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := a.NewWindow("del", "extra"); err != nil {
		t.Fatalf("new window: %v", err)
	}

	if err := a.DeleteWindow("del", 1); err != nil {
		t.Fatalf("delete window: %v", err)
	}

	if !a.tmux.SessionExists("del") {
		t.Fatal("session should still exist after deleting one of two windows")
	}

	if err := a.DeleteSession("del"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if a.tmux.SessionExists("del") {
		t.Fatal("session should be gone after delete")
	}

	if snapshotExists(dir, "del") {
		t.Fatal("snapshot should be removed after delete")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSaveAllRestoreSleepWakeup(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, dir := newTestApp(t)

	testutil.Tmux(t, "new-session", "-d", "-s", "s1")
	testutil.Tmux(t, "new-session", "-d", "-s", "s2")

	if n, err := a.SaveAll(); err != nil {
		t.Fatalf("save all: %v", err)
	} else if n != 2 {
		t.Fatalf("save all should report 2 saved sessions, got %d", n)
	}

	if !snapshotExists(dir, "s1") || !snapshotExists(dir, "s2") {
		t.Fatal("save all should snapshot both sessions")
	}

	if err := a.Sleep("s1"); err != nil {
		t.Fatalf("sleep: %v", err)
	}

	if a.tmux.SessionExists("s1") {
		t.Fatal("s1 should be asleep")
	}

	if err := a.Sleep("s1"); err == nil {
		t.Fatal("sleeping an already-asleep session should error")
	}

	if err := a.Wakeup("s1"); err != nil {
		t.Fatalf("wakeup: %v", err)
	}

	if !a.tmux.SessionExists("s1") {
		t.Fatal("s1 should be awake after wakeup")
	}

	if err := a.Wakeup("s1"); err == nil {
		t.Fatal("waking an awake session should error")
	}

	if err := a.Restore("s2", false); err != nil {
		t.Fatalf("restore existing should be tolerant: %v", err)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestBootstrapEmptyStoreNoError(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)

	if err := a.Bootstrap("last"); err != nil {
		t.Fatalf("bootstrap on empty store should be a no-op, got %v", err)
	}
}

type fakeTicker struct {
	ch chan time.Time
}

func (f *fakeTicker) Chan() <-chan time.Time { return f.ch }
func (f *fakeTicker) Stop()                  {}

//nolint:paralleltest // stubs the package-level newDaemonTicker seam
func TestRunDaemonSavesAll(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	a, dir := newTestApp(t)

	testutil.Tmux(t, "new-session", "-d", "-s", "d1")
	testutil.Tmux(t, "new-session", "-d", "-s", "d2")

	orig := newDaemonTicker
	defer func() { newDaemonTicker = orig }()

	saves := 0
	a.saveAllFn = func() error {
		saves++
		_, err := a.SaveAll()

		return err
	}

	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	close(ch)

	newDaemonTicker = func(time.Duration) daemonTicker { return &fakeTicker{ch: ch} }

	if err := a.RunDaemon(time.Minute); err != nil {
		t.Fatalf("run daemon: %v", err)
	}

	if saves != 2 {
		t.Fatalf("expected 2 daemon saves, got %d", saves)
	}

	if !snapshotExists(dir, "d1") || !snapshotExists(dir, "d2") {
		t.Fatal("daemon should have saved both sessions")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSelectWithFZFSorted(t *testing.T) {
	testutil.IsolatedTmux(t)
	testutil.RequireFZF(t)

	a, _ := newTestApp(t)

	if err := a.NewSession("only"); err != nil {
		t.Fatalf("new session: %v", err)
	}

	got, err := a.SelectWithFZFSorted(DefaultPickerSortOptions())
	if err != nil {
		t.Fatalf("select with fzf: %v", err)
	}

	if got != "only" {
		t.Fatalf("expected fzf to pick 'only', got %q", got)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestPickerSessionsMarksRestored(t *testing.T) {
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)

	if err := a.NewSession("live"); err != nil {
		t.Fatalf("new session live: %v", err)
	}

	if err := a.NewSession("dead"); err != nil {
		t.Fatalf("new session dead: %v", err)
	}

	if err := a.tmux.KillSession("dead"); err != nil {
		t.Fatalf("kill dead: %v", err)
	}

	sessions, err := a.pickerSessions(DefaultPickerSortOptions())
	if err != nil {
		t.Fatalf("picker sessions: %v", err)
	}

	restored := map[string]bool{}
	for _, sess := range sessions {
		restored[sess.Record.SessionName] = sess.Restored
	}

	if !restored["live"] {
		t.Fatal("live session should be marked restored")
	}

	if restored["dead"] {
		t.Fatal("dead session should not be marked restored")
	}
}

func TestParsePickerSortOptions(t *testing.T) {
	t.Parallel()

	if _, err := ParsePickerSortOptions("name", "index"); err != nil {
		t.Fatalf("valid sort options: %v", err)
	}

	if _, err := ParsePickerSortOptions("bogus", ""); err == nil {
		t.Fatal("expected error for bogus sort field")
	}
}

func TestMergeLastAttached(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	records := []snapshot.Record{
		{SessionName: "fresher-attach", LastAccessed: base},
		{SessionName: "fresher-stored", LastAccessed: base.Add(2 * time.Hour)},
		{SessionName: "not-live", LastAccessed: base},
	}

	attached := map[string]time.Time{
		"fresher-attach": base.Add(time.Hour),
		"fresher-stored": base.Add(time.Hour),
	}

	mergeLastAttached(records, attached)

	if got := records[0].LastAccessed; !got.Equal(base.Add(time.Hour)) {
		t.Fatalf("fresher-attach should take the newer attach time, got %v", got)
	}

	if got := records[1].LastAccessed; !got.Equal(base.Add(2 * time.Hour)) {
		t.Fatalf("fresher-stored should keep the newer stored time, got %v", got)
	}

	if got := records[2].LastAccessed; !got.Equal(base) {
		t.Fatalf("not-live should be unchanged, got %v", got)
	}

	mergeLastAttached(records, nil)
}
