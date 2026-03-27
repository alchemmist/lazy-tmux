//go:build integration

package app

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

func TestWakeupIntegrationRestoresSession(t *testing.T) {
	logFile := "/tmp/lazy-tmux-wakeup-test-log"
	os.RemoveAll(logFile) // Clean up any prior test data

	tmuxBin := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  echo "has-session" >> "`+logFile+`"
  exit 1
fi
if [ "$1" = "new-session" ]; then
  echo "new-session" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "send-keys" ]; then
  echo "send-keys" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-layout" ]; then
  echo "select-layout" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-window" ]; then
  echo "select-window" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-pane" ]; then
  echo "select-pane" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "display-message" ]; then
  echo "display-message" >> "`+logFile+`"
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
  echo "list-windows" >> "`+logFile+`"
  printf "0\037main\037layout\0371\n"
  exit 0
fi
if [ "$1" = "list-panes" ]; then
  echo "list-panes" >> "`+logFile+`"
  printf "0\037/home/user\037zsh\0371\037111\037\n"
  exit 0
fi
echo "unknown: $@" >> "`+logFile+`"
exit 1
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
		t.Fatalf(
			"wakeup command failed: %v\nstdout: %s\nstderr: %s",
			err,
			out.String(),
			errOut.String(),
		)
	}

	// Verify session still exists in store after wakeup
	loaded, err := s.LoadSession("myapp")
	if err != nil {
		t.Fatalf("expected session to remain in store: %v", err)
	}
	if loaded.SessionName != "myapp" {
		t.Fatalf("expected session name 'myapp', got '%s'", loaded.SessionName)
	}

	// Verify expected tmux commands were invoked
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected tmux commands to be logged: %v", err)
	}
	logContent := string(logData)
	if !strings.Contains(logContent, "has-session") {
		t.Fatalf("expected 'has-session' to be called, log: %s", logContent)
	}
	if !strings.Contains(logContent, "new-session") {
		t.Fatalf("expected 'new-session' to be called, log: %s", logContent)
	}
	if !strings.Contains(logContent, "select-window") {
		t.Fatalf("expected 'select-window' to be called, log: %s", logContent)
	}
	if !strings.Contains(logContent, "select-pane") {
		t.Fatalf("expected 'select-pane' to be called, log: %s", logContent)
	}
}

func TestSleepIntegrationSavesAndClosesSession(t *testing.T) {
	logFile := "/tmp/lazy-tmux-sleep-test-log"
	os.RemoveAll(logFile) // Clean up any prior test data

	tmuxBin := writeFakeTmuxForApp(t, `
if [ "$1" = "has-session" ]; then
  echo "has-session" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "kill-session" ]; then
  echo "kill-session" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "send-keys" ]; then
  echo "send-keys" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-layout" ]; then
  echo "select-layout" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-window" ]; then
  echo "select-window" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "select-pane" ]; then
  echo "select-pane" >> "`+logFile+`"
  exit 0
fi
if [ "$1" = "display-message" ]; then
  echo "display-message" >> "`+logFile+`"
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
  echo "list-windows" >> "`+logFile+`"
  printf "0\037editor\037layout\0371\n"
  exit 0
fi
if [ "$1" = "list-panes" ]; then
  echo "list-panes" >> "`+logFile+`"
  printf "0\037/home/user/project\037nvim\0371\037222\037\n"
  exit 0
fi
if [ "$1" = "capture-pane" ]; then
  echo "capture-pane" >> "`+logFile+`"
  printf "nvim session content\n"
  exit 0
fi
echo "unknown: $@" >> "`+logFile+`"
exit 1
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
		t.Fatalf(
			"sleep command failed: %v\nstdout: %s\nstderr: %s",
			err,
			out.String(),
			errOut.String(),
		)
	}

	// Verify session still exists in store after sleep
	loaded, err := s.LoadSession("workspace")
	if err != nil {
		t.Fatalf("expected session to remain in store: %v", err)
	}
	if loaded.SessionName != "workspace" {
		t.Fatalf("expected session name 'workspace', got '%s'", loaded.SessionName)
	}

	// Verify expected tmux commands were invoked
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected tmux commands to be logged: %v", err)
	}
	logContent := string(logData)
	if !strings.Contains(logContent, "has-session") {
		t.Fatalf("expected 'has-session' to be called, log: %s", logContent)
	}
	if !strings.Contains(logContent, "kill-session") {
		t.Fatalf("expected 'kill-session' to be called, log: %s", logContent)
	}
}

func TestWakeupHelpWorks(t *testing.T) {
	bin := buildLazyTmuxBinary(t)

	cmd := exec.Command(bin, "wakeup", "-h")
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)
	if !strings.Contains(outputStr, "session to wakeup") {
		t.Fatalf("expected help text in output, got: %s", outputStr)
	}
}

func TestSleepHelpWorks(t *testing.T) {
	bin := buildLazyTmuxBinary(t)

	cmd := exec.Command(bin, "sleep", "-h")
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)
	if !strings.Contains(outputStr, "session to sleep") {
		t.Fatalf("expected help text in output, got: %s", outputStr)
	}
}
