package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestDeleteSessionOnlineRemovesSnapshot(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "kill-session" ]; then
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

	if err := a.DeleteSession("demo"); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}

	if _, err := a.store.LoadSession("demo"); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected session to be deleted, got %v", err)
	}
}

func TestRenameWindowUpdatesSnapshotOffline(t *testing.T) {
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
			{Index: 1, Name: "old", Panes: []snapshot.Pane{{Index: 0}}},
		},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.RenameWindow("demo", 1, "new"); err != nil {
		t.Fatalf("RenameWindow error: %v", err)
	}

	snap, err := a.store.LoadSession("demo")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if snap.Windows[0].Name != "new" {
		t.Fatalf("expected window name to be updated, got %q", snap.Windows[0].Name)
	}
}

func TestNewWindowOfflineAddsWindowWithDefaultName(t *testing.T) {
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
			{Index: 0, Name: "a", Panes: []snapshot.Pane{{Index: 0}}},
			{Index: 2, Name: "b", Panes: []snapshot.Pane{{Index: 0}}},
		},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.NewWindow("demo", ""); err != nil {
		t.Fatalf("NewWindow error: %v", err)
	}

	snap, err := a.store.LoadSession("demo")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if len(snap.Windows) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(snap.Windows))
	}

	last := snap.Windows[2]
	if last.Index != 3 || last.Name != "window-3" {
		t.Fatalf("unexpected new window: %+v", last)
	}
}

func TestRenameSessionValidations(t *testing.T) {
	a := &App{}
	if err := a.RenameSession("demo", ""); err == nil {
		t.Fatal("expected error for empty name")
	}

	if err := a.RenameSession("", "demo"); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestRenameSessionNoOpSameName(t *testing.T) {
	a := &App{}
	if err := a.RenameSession("demo", "demo"); err != nil {
		t.Fatalf("expected nil for same name, got %v", err)
	}
}

func TestDeleteWindowReportsMissingWindow(t *testing.T) {
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
		Windows:     []snapshot.Window{{Index: 1, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := a.DeleteWindow("demo", 2); err == nil {
		t.Fatal("expected error when window is missing")
	}
}

func TestDeleteWindowOnlineDeletesSessionWhenLastWindow(t *testing.T) {
	state := filepath.Join(t.TempDir(), "count")
	fake := writeFakeTmuxForApp(t, `
count_file="$COUNT_FILE"
if [ "$1" = "has-session" ]; then
  if [ ! -f "$count_file" ]; then
    echo 1 > "$count_file"
    exit 0
  fi
  exit 1
fi
if [ "$1" = "kill-window" ]; then
  exit 0
fi
exit 0
`)

	t.Setenv("COUNT_FILE", state)

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

	if _, err := a.store.LoadSession("demo"); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected session to be deleted, got %v", err)
	}
}

func TestRenameWindowValidatesName(t *testing.T) {
	a := &App{}
	if err := a.RenameWindow("demo", 1, " "); err == nil {
		t.Fatal("expected error for empty window name")
	}
}

func TestNewSessionValidatesName(t *testing.T) {
	a := &App{}
	if err := a.NewSession(" "); err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestNewSessionRejectsExistingInStore(t *testing.T) {
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

	if err := a.NewSession("demo"); err == nil {
		t.Fatal("expected error when session exists in store")
	}
}
