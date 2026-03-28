package app

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestDeleteWindowOfflineRemovesFromSnapshot(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 1, Panes: []snapshot.Pane{{Index: 0}}},
			{Index: 3, Panes: []snapshot.Pane{{Index: 0}}},
		},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.DeleteWindow("demo", 3); err != nil {
		t.Fatalf("DeleteWindow error: %v", err)
	}

	snap, err := a.store.LoadSession("demo")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(snap.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(snap.Windows))
	}
	if snap.Windows[0].Index != 1 {
		t.Fatalf("expected remaining window index 1, got %d", snap.Windows[0].Index)
	}
}

func TestDeleteWindowOfflineDeletesSessionWhenLastWindow(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.DeleteWindow("demo", 0); err != nil {
		t.Fatalf("DeleteWindow error: %v", err)
	}

	_, err := a.store.LoadSession("demo")
	if err == nil {
		t.Fatal("expected session to be deleted")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestRenameSessionNoOpOnNormalizedCollision(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "foo/bar",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.RenameSession("foo/bar", "foo_bar"); err != nil {
		t.Fatalf("RenameSession error: %v", err)
	}

	recs, err := a.store.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(recs) != 1 || recs[0].SessionName != "foo/bar" {
		t.Fatalf("unexpected records: %+v", recs)
	}
}

func TestRenameSessionDestinationExists(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	for _, name := range []string{"alpha", "beta"} {
		if err := a.store.SaveSession(snapshot.SessionSnapshot{
			Version:     snapshot.FormatVersion,
			SessionName: name,
			CapturedAt:  time.Now().UTC(),
			Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		}); err != nil {
			t.Fatalf("save session %s: %v", name, err)
		}
	}

	if err := a.RenameSession("alpha", "beta"); err == nil {
		t.Fatal("expected rename collision error")
	}

	recs, err := a.store.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
}

func TestWakeupFailsOnAlreadyRunningSession(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 0
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.Wakeup("demo"); err == nil {
		t.Fatal("expected error for already running session")
	}
}

func TestWakeupFailsOnNonexistentSession(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}

	if err := a.Wakeup("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestSleepFailsOnNonrunningSession(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.Sleep("demo"); err == nil {
		t.Fatal("expected error for nonrunning session")
	}
}

func TestSleepKillsRunningSession(t *testing.T) {
	tempDir := t.TempDir()
	markerFile := tempDir + "/kill-marker"
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "kill-session" ]; then
  echo "killed" >> "`+markerFile+`"
  exit 0
fi
if [ "$1" = "display-message" ]; then
  if echo "$*" | grep -q "#{window_index}"; then
    printf "0\0370\n"
    exit 0
  fi
  if echo "$*" | grep -q "#{pane_tty}"; then
    printf "/dev/pts/1\n"
    exit 0
  fi
  if echo "$*" | grep -q "#{socket_path}"; then
    printf "/tmp/tmux.sock\n"
    exit 0
  fi
  if echo "$*" | grep -q "#S"; then
    printf "demo\n"
    exit 0
  fi
  printf "0\n"
  exit 0
fi
if [ "$1" = "list-windows" ]; then
  printf "0\037main\037layout\0371\n"
  exit 0
fi
if [ "$1" = "list-panes" ]; then
  printf "0\037/tmp\037zsh\0371\037111\037\n"
  exit 0
fi
if [ "$1" = "capture-pane" ]; then
  printf "test\n"
  exit 0
fi
exit 0
`)
	dataDir := t.TempDir()
	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.Sleep("demo"); err != nil {
		t.Fatalf("Sleep error: %v", err)
	}

	// Session should still be in store after sleep
	_, err := a.store.LoadSession("demo")
	if err != nil {
		t.Fatalf("expected session to remain in store: %v", err)
	}

	// Verify that kill-session was actually executed
	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatalf("expected kill-session to be called (marker file missing): %v", err)
	}
	if !strings.Contains(string(data), "killed") {
		t.Fatalf("expected 'killed' in marker file, got: %s", string(data))
	}
}
