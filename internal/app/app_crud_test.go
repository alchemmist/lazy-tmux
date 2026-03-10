package app

import (
	"os"
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
