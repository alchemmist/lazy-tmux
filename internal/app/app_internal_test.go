package app

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

type workingAntexIntegration struct{}

func (workingAntexIntegration) Name() string { return "antex" }
func (workingAntexIntegration) Matches(pane snapshot.Pane) bool {
	return pane.CurrentCmd == "antex"
}

func (workingAntexIntegration) Capture(snapshot.Pane) (map[string]string, error) {
	return map[string]string{}, nil
}
func (workingAntexIntegration) RestoreCommand(snapshot.Pane, map[string]string) string { return "" }
func (workingAntexIntegration) Status(snapshot.Pane) (integration.Status, bool) {
	return integration.StatusWorking, true
}

func waitClient(t *testing.T, want string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		got := strings.TrimSpace(
			testutil.Tmux(t, "list-clients", "-F", "#{session_name}:#{window_index}"),
		)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("clients %q, want %q", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func attachControl(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cmd := exec.CommandContext(ctx, "tmux", "-C", "attach-session", "-t", "="+name)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); cancel(); _ = cmd.Wait() })
	waitClient(t, name+":0")
}

func TestRestoreTargetInsideTmux(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	a, _ := newTestApp(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "driver")
	attachControl(t, "driver")
	t.Setenv(
		"TMUX",
		strings.TrimSpace(
			testutil.Tmux(t, "display-message", "-p", "-t", "=driver:0", "#{socket_path},#{pid},0"),
		),
	)
	target := PickerTarget{SessionName: "saved"}
	if err := a.NewSession("saved"); err != nil {
		t.Fatal(err)
	}
	if err := a.Sleep("saved"); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreTarget(target, true); err != nil {
		t.Fatal(err)
	}
	waitClient(t, "saved:0")
	testutil.Tmux(t, "switch-client", "-t", "=driver")
	testutil.Tmux(t, "set-option", "-w", "-t", "=saved:0", "window-size", "manual")
	if err := a.RestoreTargetInteractive(target); err != nil {
		t.Fatal(err)
	}
	waitClient(t, "saved:0")
	if got := strings.TrimSpace(
		testutil.Tmux(t, "show-options", "-wv", "-t", "=saved:0", "window-size"),
	); got != "manual" {
		t.Fatalf("live size mode: %q", got)
	}
}

func TestRestoreTargetChild(t *testing.T) {
	t.Parallel()
	args := flag.Args()
	if len(args) != 3 || args[0] != "handoff" {
		t.Skip("subprocess entry point")
	}
	a := New(config.Config{TmuxBin: "tmux", DataDir: args[1]})
	index := 1
	target := PickerTarget{SessionName: "saved", WindowIndex: &index}
	var err error
	if args[2] == "interactive" {
		err = a.RestoreTargetInteractive(target)
	} else {
		err = a.RestoreTarget(target, true)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(args[1], "returned"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreTargetOutsideTmux(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	a, dir := newTestApp(t)
	if err := a.NewSession("saved"); err != nil {
		t.Fatal(err)
	}
	if err := a.NewWindow("saved", "second"); err != nil {
		t.Fatal(err)
	}
	if err := a.Sleep("saved"); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := func(mode string) {
		testutil.Tmux(
			t,
			"new-session",
			"-d",
			"-s",
			mode,
			"env",
			"-u",
			"TMUX",
			exe,
			"-test.run=^TestRestoreTargetChild$",
			"--",
			"handoff",
			dir,
			mode,
		)
	}
	start("cli")
	for deadline := time.Now().Add(5 * time.Second); ; {
		if _, err := os.Stat(filepath.Join(dir, "returned")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CLI did not return after restore")
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitClient(t, "")
	start("interactive")
	waitClient(t, "saved:1")
}

func TestQuickPickerSessionsIncludesLiveWithoutSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)
	if err := a.store.SaveSession(snapshot.SessionSnapshot{SessionName: "saved"}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "live-only")

	sessions, err := a.quickPickerSessions()
	if err != nil {
		t.Fatalf("quick picker sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v, want saved and live-only", sessions)
	}

	byName := make(map[string]picker.QuickSession, len(sessions))
	for _, session := range sessions {
		byName[session.Name] = session
	}
	if byName["saved"].Restored {
		t.Fatal("saved snapshot should not be marked restored")
	}
	if !byName["live-only"].Restored || !byName["live-only"].Current {
		t.Fatalf("live-only session flags = %+v", byName["live-only"])
	}
}

func TestQuickPickerSessionsHaveStableNameOrder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)
	for _, session := range []snapshot.SessionSnapshot{
		{SessionName: "charlie", CapturedAt: time.Now().Add(time.Hour)},
		{SessionName: "alpha", CapturedAt: time.Now()},
	} {
		if err := a.store.SaveSession(session); err != nil {
			t.Fatalf("save snapshot: %v", err)
		}
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "bravo")
	testutil.Tmux(t, "new-session", "-d", "-s", "delta")
	attachControl(t, "delta")

	sessions, err := a.quickPickerSessions()
	if err != nil {
		t.Fatalf("quick picker sessions: %v", err)
	}
	names := make([]string, 0, len(sessions))
	for _, session := range sessions {
		names = append(names, session.Name)
	}
	if !slices.Equal(names, []string{"delta", "bravo", "charlie", "alpha"}) {
		t.Fatalf("session order = %v", names)
	}
}

func TestOpenQuickSessionSwitchesLiveSessionWithoutSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "driver")
	testutil.Tmux(t, "new-session", "-d", "-s", "live-only")
	attachControl(t, "driver")
	t.Setenv(
		"TMUX",
		strings.TrimSpace(
			testutil.Tmux(t, "display-message", "-p", "-t", "=driver:0", "#{socket_path},#{pid},0"),
		),
	)

	if err := a.OpenQuickSession("live-only"); err != nil {
		t.Fatalf("open quick session: %v", err)
	}
	waitClient(t, "live-only:0")
}

func TestOpenQuickSessionUpdatesLastUsed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)
	if err := a.store.SaveSession(snapshot.SessionSnapshot{SessionName: "live"}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "live")
	attachControl(t, "live")
	t.Setenv(
		"TMUX",
		strings.TrimSpace(
			testutil.Tmux(t, "display-message", "-p", "-t", "=live:0", "#{socket_path},#{pid},0"),
		),
	)

	if err := a.OpenQuickSession("live"); err != nil {
		t.Fatalf("open quick session: %v", err)
	}
	records, err := a.store.ListRecords()
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].LastAccessed.IsZero() {
		t.Fatalf("last-used timestamp was not updated: %+v", records)
	}
}

func TestQuickPickerSessionMarksWorkingAntex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)

	a, _ := newTestApp(t)
	a.integrations = integration.NewRegistry(workingAntexIntegration{})
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "working",
		CapturedAt:  time.Now(),
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{{
			Index:      0,
			Name:       "antex",
			Layout:     "",
			IsActive:   true,
			ActivePane: 0,
			Panes: []snapshot.Pane{{
				Index:       0,
				CurrentPath: "/workspace",
				CurrentCmd:  "antex",
				RestoreCmd:  "",
				Scrollback:  nil,
				IsActive:    true,
				Meta:        nil,
			}},
		}},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "working")

	sessions, err := a.quickPickerSessions()
	if err != nil {
		t.Fatalf("quick picker sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Working {
		t.Fatalf("Antex status blocked the initial session list: %+v", sessions)
	}
	working := a.quickWorkingSessions(sessions)
	if !working["working"] {
		t.Fatalf("working Antex session was not detected: %+v", working)
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

func TestRunDaemonSavesAll(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	testutil.IsolatedTmux(t)
	a, dir := newTestApp(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "first")
	run := func(interval time.Duration) context.CancelFunc {
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := a.RunDaemon(ctx, interval); err != nil {
				t.Errorf("daemon: %v", err)
			}
		}()
		stop := func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("daemon did not stop")
			}
		}
		t.Cleanup(stop)

		return stop
	}
	waitForSnapshot := func(name string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if snapshotExists(dir, name) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("missing snapshot: %s", name)
	}
	stop := run(time.Hour)
	waitForSnapshot("first")
	stop()
	testutil.Tmux(t, "new-session", "-d", "-s", "second")
	run(50 * time.Millisecond)
	waitForSnapshot("second")
	testutil.Tmux(t, "new-session", "-d", "-s", "third")
	waitForSnapshot("third")
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

func TestPickerSnapshotCacheReusesAndInvalidatesMetadata(t *testing.T) {
	t.Parallel()

	a, dir := newTestApp(t)
	first := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "cached",
		CapturedAt:  time.Unix(1, 0),
		Windows:     []snapshot.Window{{Index: 0, Name: "first"}},
	}
	if err := a.store.SaveSession(first); err != nil {
		t.Fatalf("save first: %v", err)
	}
	records, err := a.store.ListRecords()
	if err != nil {
		t.Fatalf("list first records: %v", err)
	}
	if _, err = a.pickerSnapshot(records[0]); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if err = os.Remove(filepath.Join(dir, "sessions", "cached.json")); err != nil {
		t.Fatalf("remove cached fixture: %v", err)
	}
	if _, err = a.pickerSnapshot(records[0]); err != nil {
		t.Fatalf("unchanged record was reloaded: %v", err)
	}

	second := first
	second.CapturedAt = time.Unix(2, 0)
	second.Windows = []snapshot.Window{{Index: 0, Name: "second"}}
	if err = a.store.SaveSession(second); err != nil {
		t.Fatalf("save second: %v", err)
	}
	records, err = a.store.ListRecords()
	if err != nil {
		t.Fatalf("list second records: %v", err)
	}
	loaded, err := a.pickerSnapshot(records[0])
	if err != nil {
		t.Fatalf("reload changed record: %v", err)
	}
	if loaded.Windows[0].Name != "second" {
		t.Fatalf("stale cached snapshot: %+v", loaded.Windows)
	}
}

func TestPickerSnapshotCoalescesConcurrentCacheMisses(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t)
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "shared",
		CapturedAt:  time.Unix(1, 0),
		Windows:     []snapshot.Window{{Index: 0, Name: "window"}},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	records, err := a.store.ListRecords()
	if err != nil {
		t.Fatalf("list records: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 32)
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			<-start
			_, loadErr := a.pickerSnapshot(records[0])
			errs <- loadErr
		})
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("load snapshot: %v", err)
		}
	}
	if a.pickerCacheMisses != 1 {
		t.Fatalf("cache misses = %d, want 1", a.pickerCacheMisses)
	}
}
