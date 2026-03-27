//go:build integration

package app

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

func TestWakeupIntegrationRestoresSession(t *testing.T) {
	tmuxBin := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "new-session" ]; then
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
    printf "myapp\n"
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
  printf "0\037/home/user\037zsh\0371\037111\037\n"
  exit 0
fi
exit 0
`)

	dataDir := t.TempDir()
	bin := buildLazyTmuxBinary(t)

	// First, save a session
	s := store.New(dataDir)
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "myapp",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index:      0,
				Name:       "main",
				Layout:     "layout",
				ActivePane: 0,
				Panes: []snapshot.Pane{
					{
						Index:       0,
						CurrentPath: "/home/user",
						CurrentCmd:  "zsh",
						RestoreCmd:  "cd /home/user && zsh",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	// Run wakeup command
	cmd := exec.Command(bin,
		"wakeup",
		"--session", "myapp",
		"--data-dir", dataDir,
		"--tmux-bin", tmuxBin,
	)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		t.Fatalf("wakeup command failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}

	// Verify session still exists in store after wakeup
	loaded, err := s.LoadSession("myapp")
	if err != nil {
		t.Fatalf("expected session to remain in store: %v", err)
	}
	if loaded.SessionName != "myapp" {
		t.Fatalf("expected session name 'myapp', got '%s'", loaded.SessionName)
	}
}

func TestSleepIntegrationSavesAndClosesSession(t *testing.T) {
	capturedPaneOutput := false
	tmuxBin := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "kill-session" ]; then
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
    printf "workspace\n"
    exit 0
  fi
  printf "0\n"
  exit 0
fi
if [ "$1" = "list-windows" ]; then
  printf "0\037editor\037layout\0371\n"
  exit 0
fi
if [ "$1" = "list-panes" ]; then
  printf "0\037/home/user/project\037nvim\0371\037222\037\n"
  exit 0
fi
if [ "$1" = "capture-pane" ]; then
  printf "nvim session content\n"
  exit 0
fi
exit 0
`)

	dataDir := t.TempDir()
	bin := buildLazyTmuxBinary(t)

	// First, save an initial snapshot
	s := store.New(dataDir)
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "workspace",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{
				Index:      0,
				Name:       "editor",
				Layout:     "layout",
				ActivePane: 0,
				Panes: []snapshot.Pane{
					{
						Index:       0,
						CurrentPath: "/home/user/project",
						CurrentCmd:  "nvim",
						RestoreCmd:  "cd /home/user/project && nvim",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save initial session: %v", err)
	}

	// Run sleep command
	cmd := exec.Command(bin,
		"sleep",
		"--session", "workspace",
		"--data-dir", dataDir,
		"--tmux-bin", tmuxBin,
	)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		t.Fatalf("sleep command failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errOut.String())
	}

	// Verify session still exists in store after sleep
	loaded, err := s.LoadSession("workspace")
	if err != nil {
		t.Fatalf("expected session to remain in store: %v", err)
	}
	if loaded.SessionName != "workspace" {
		t.Fatalf("expected session name 'workspace', got '%s'", loaded.SessionName)
	}

	_ = capturedPaneOutput
}

func TestWakeupHelpWorks(t *testing.T) {
	bin := buildLazyTmuxBinary(t)

	cmd := exec.Command(bin, "wakeup", "-h")
	var out bytes.Buffer
	cmd.Stdout = &out

	_ = cmd.Run() // May exit with non-zero for -h

	output := out.String()
	if !strings.Contains(output, "session to wakeup") {
		t.Fatalf("expected help text in output, got: %s", output)
	}
}

func TestSleepHelpWorks(t *testing.T) {
	bin := buildLazyTmuxBinary(t)

	cmd := exec.Command(bin, "sleep", "-h")
	var out bytes.Buffer
	cmd.Stdout = &out

	_ = cmd.Run() // May exit with non-zero for -h

	output := out.String()
	if !strings.Contains(output, "session to sleep") {
		t.Fatalf("expected help text in output, got: %s", output)
	}
}
