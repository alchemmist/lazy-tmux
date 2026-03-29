package app

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestSaveCurrentCreatesSnapshot(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "display-message" ]; then
  if [ "$2" = "-p" ] && [ "$3" = "#S" ]; then
    echo "demo"
    exit 0
  fi
  printf "0\0370\n"
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
exit 0
`)

	app := &App{
		cfg:   config.Config{Scrollback: config.ScrollbackConfig{Enabled: false}},
		store: store.New(t.TempDir()),
		tmux:  tmux.NewClient(fake),
	}

	if err := app.SaveCurrent(); err != nil {
		t.Fatalf("SaveCurrent error: %v", err)
	}

	if _, err := app.store.LoadSession("demo"); err != nil {
		t.Fatalf("expected snapshot saved, got %v", err)
	}
}

func TestSaveAllSavesEachSession(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "list-sessions" ]; then
  printf "alpha\nbeta\n"
  exit 0
fi
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "display-message" ]; then
  printf "0\0370\n"
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
exit 0
`)

	app := &App{
		cfg:   config.Config{Scrollback: config.ScrollbackConfig{Enabled: false}},
		store: store.New(t.TempDir()),
		tmux:  tmux.NewClient(fake),
	}

	if err := app.SaveAll(); err != nil {
		t.Fatalf("SaveAll error: %v", err)
	}

	for _, name := range []string{"alpha", "beta"} {
		if _, err := app.store.LoadSession(name); err != nil {
			t.Fatalf("expected %s snapshot saved, got %v", name, err)
		}
	}
}

func TestRestoreReturnsErrorOnEmptySession(t *testing.T) {
	app := &App{}
	if err := app.Restore(" ", false); err == nil {
		t.Fatal("expected error for empty session name")
	}
}

func TestRestoreMarksAccessed(t *testing.T) {
	logPath := t.TempDir() + "/tmux.log"
	fake := writeFakeTmuxForApp(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 0
fi
exit 0
`)

	t.Setenv("TMUX_LOG", logPath)

	dataDir := t.TempDir()

	app := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := app.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	if err := app.Restore("demo", false); err != nil {
		t.Fatalf("Restore error: %v", err)
	}

	rec, err := app.store.LatestRecord()
	if err != nil {
		t.Fatalf("LatestRecord: %v", err)
	}

	if rec.SessionName != "demo" || rec.LastAccessed.IsZero() {
		t.Fatalf("expected last accessed to be updated, got %+v", rec)
	}
}
