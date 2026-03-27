package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestSelectWithFZFNoRecords(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}

	_, err := a.SelectWithFZF()
	if err == nil {
		t.Fatal("expected error when there are no records")
	}

	if !strings.Contains(err.Error(), "no saved sessions found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapLastWithoutRecordsReturnsNil(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}
	if err := a.Bootstrap("last"); err != nil {
		t.Fatalf("expected nil when no records exist, got %v", err)
	}
}

func TestAcquireLockIsExclusive(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	unlock1, err := acquireLock("/tmp/tmux.sock")
	if err != nil {
		t.Fatalf("first lock should succeed, got %v", err)
	}
	defer unlock1()

	_, err = acquireLock("/tmp/tmux.sock")
	if err == nil {
		t.Fatal("second lock should fail")
	}

	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected lock error: %v", err)
	}

	unlock1()

	unlock2, err := acquireLock("/tmp/tmux.sock")
	if err != nil {
		t.Fatalf("lock after unlock should succeed, got %v", err)
	}

	if unlock2 == nil {
		t.Fatal("unlock function must not be nil")
	}

	unlock2()
}

func TestBootstrapEmptyAliasWithoutRecordsReturnsNil(t *testing.T) {
	a := &App{store: store.New(t.TempDir())}
	if err := a.Bootstrap("  "); err != nil {
		t.Fatalf("expected nil for empty alias when no records exist, got %v", err)
	}
}

func TestRestoreTargetSwitchesToSelectedWindowWhenSessionExists(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmuxForApp(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "switch-client" ]; then
  exit 0
fi
exit 0
`)

	t.Setenv("TMUX_LOG", logPath)
	t.Setenv("TMUX", "1")

	dataDir := t.TempDir()

	a := &App{
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}
	if err := a.store.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows:     []snapshot.Window{{Index: 3, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	idx := 3
	if err := a.RestoreTarget(
		PickerTarget{SessionName: "demo", WindowIndex: &idx},
		true,
	); err != nil {
		t.Fatalf("RestoreTarget error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, "switch-client -t =demo:3") {
		t.Fatalf("expected window-specific switch target, got:\n%s", out)
	}
}

func TestRestoreTargetDoesNotSwitchWhenSwitchDisabled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	fake := writeFakeTmuxForApp(t, `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "switch-client" ]; then
  exit 0
fi
exit 0
`)

	t.Setenv("TMUX_LOG", logPath)
	t.Setenv("TMUX", "1")

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

	if err := a.RestoreTarget(PickerTarget{SessionName: "demo"}, false); err != nil {
		t.Fatalf("RestoreTarget error: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	out := string(b)
	if strings.Contains(out, "switch-client -t") {
		t.Fatalf("switch-client must not be called when switch=false, got:\n%s", out)
	}
}

func TestSaveSessionCapturesShellScrollbackWhenEnabled(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
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
if [ "$1" = "capture-pane" ]; then
  printf "echo hi\nhi\n"
  exit 0
fi
exit 0
`)

	dataDir := t.TempDir()
	a := &App{
		cfg: config.Config{
			Scrollback: config.ScrollbackConfig{Enabled: true, Lines: 200},
		},
		store: store.New(dataDir),
		tmux:  tmux.NewClient(fake),
	}

	if err := a.SaveSession("demo"); err != nil {
		t.Fatalf("SaveSession error: %v", err)
	}

	loaded, err := a.store.LoadSession("demo")
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}

	sb := loaded.Windows[0].Panes[0].Scrollback
	if sb == nil {
		t.Fatal("expected shell pane scrollback to be captured")
	}

	if !strings.Contains(sb.Content, "echo hi") {
		t.Fatalf("unexpected scrollback content: %q", sb.Content)
	}
}

func writeFakeTmuxForApp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	return path
}
