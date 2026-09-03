package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

type recordingTmux struct {
	tmuxClient

	inside   bool
	exists   bool
	switched string
	synced   string
	attached string
}

func (r *recordingTmux) RestoreSession(context.Context, snapshot.SessionSnapshot) error {
	return nil
}
func (r *recordingTmux) InsideTmux() bool          { return r.inside }
func (r *recordingTmux) SessionExists(string) bool { return r.exists }

type quickRecordingTmux struct {
	recordingTmux

	sessions []string
	current  string
	attached map[string]time.Time
}

func (r *quickRecordingTmux) ListSessions() ([]string, error)            { return r.sessions, nil }
func (r *quickRecordingTmux) CurrentSession() (string, error)            { return r.current, nil }
func (r *quickRecordingTmux) SessionsLastAttached() map[string]time.Time { return r.attached }

type workingCodexIntegration struct{}

func (workingCodexIntegration) Name() string { return "codex" }
func (workingCodexIntegration) Matches(pane snapshot.Pane) bool {
	return pane.CurrentCmd == "codex"
}

func (workingCodexIntegration) Capture(snapshot.Pane) (map[string]string, error) {
	return map[string]string{}, nil
}
func (workingCodexIntegration) RestoreCommand(snapshot.Pane, map[string]string) string { return "" }
func (workingCodexIntegration) Status(snapshot.Pane) (integration.Status, bool) {
	return integration.StatusWorking, true
}

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

func (r *recordingTmux) SynchronizeWindowSize(target string) error {
	r.synced = target

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
		wantSynced   string
		wantAttached string
		exists       bool
	}{
		{name: "cli inside tmux switches", inside: true, wantSwitched: "s1", wantSynced: "s1"},
		{
			name:         "picker inside tmux switches",
			interactive:  true,
			inside:       true,
			wantSwitched: "s1",
			wantSynced:   "s1",
		},
		{name: "live session keeps its size mode", inside: true, exists: true, wantSwitched: "s1"},

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

			fake := &recordingTmux{inside: tc.inside, exists: tc.exists}
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
			if fake.synced != tc.wantSynced {
				t.Fatalf("synced = %q, want %q", fake.synced, tc.wantSynced)
			}

			if fake.attached != tc.wantAttached {
				t.Fatalf("attached = %q, want %q", fake.attached, tc.wantAttached)
			}
		})
	}
}

func TestQuickPickerSessionsIncludesLiveWithoutSnapshot(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t)
	if err := a.store.SaveSession(snapshot.SessionSnapshot{SessionName: "saved"}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	a.tmux = &quickRecordingTmux{
		recordingTmux: recordingTmux{},
		sessions:      []string{"live-only"},
		current:       "live-only",
		attached:      nil,
	}

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
	t.Parallel()

	a, _ := newTestApp(t)
	for _, session := range []snapshot.SessionSnapshot{
		{SessionName: "charlie", CapturedAt: time.Now().Add(time.Hour)},
		{SessionName: "alpha", CapturedAt: time.Now()},
	} {
		if err := a.store.SaveSession(session); err != nil {
			t.Fatalf("save snapshot: %v", err)
		}
	}
	a.tmux = &quickRecordingTmux{
		recordingTmux: recordingTmux{},
		sessions:      []string{"delta", "bravo"},
		current:       "delta",
		attached: map[string]time.Time{
			"bravo": time.Now(),
			"delta": time.Now().Add(time.Hour),
		},
	}

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
	t.Parallel()

	a, _ := newTestApp(t)
	fake := &recordingTmux{inside: true, exists: true}
	a.tmux = fake

	if err := a.OpenQuickSession("live-only"); err != nil {
		t.Fatalf("open quick session: %v", err)
	}
	if fake.switched != "live-only" {
		t.Fatalf("switched = %q, want live-only", fake.switched)
	}
}

func TestOpenQuickSessionUpdatesLastUsed(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t)
	if err := a.store.SaveSession(snapshot.SessionSnapshot{SessionName: "live"}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	a.tmux = &recordingTmux{inside: true, exists: true}

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

func TestQuickPickerSessionMarksWorkingCodex(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t)
	a.integrations = integration.NewRegistry(workingCodexIntegration{})
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "working",
		CapturedAt:  time.Now(),
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{{
			Index:      0,
			Name:       "codex",
			Layout:     "",
			IsActive:   true,
			ActivePane: 0,
			Panes: []snapshot.Pane{{
				Index:       0,
				CurrentPath: "/workspace",
				CurrentCmd:  "codex",
				RestoreCmd:  "",
				Scrollback:  nil,
				IsActive:    true,
				Meta:        nil,
			}},
		}},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	a.tmux = &quickRecordingTmux{
		recordingTmux: recordingTmux{},
		sessions:      []string{"working"},
		current:       "working",
		attached:      nil,
	}

	sessions, err := a.quickPickerSessions()
	if err != nil {
		t.Fatalf("quick picker sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Working {
		t.Fatalf("Codex status blocked the initial session list: %+v", sessions)
	}
	working := a.quickWorkingSessions(sessions)
	if !working["working"] {
		t.Fatalf("working Codex session was not detected: %+v", working)
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
