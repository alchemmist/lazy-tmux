//go:build integration

package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonIntegrationWritesSnapshots(t *testing.T) {
	body := `
echo "$*" >> "$TMUX_LOG"
if [ "$1" = "has-session" ]; then
  exit 0
fi
if [ "$1" = "list-sessions" ]; then
  printf "demo\n"
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
  printf "echo hi\nhi\n"
  exit 0
fi
exit 0
`
	logPath := filepath.Join(t.TempDir(), "tmux.log")
	t.Setenv("TMUX_LOG", logPath)
	tmuxBin := writeFakeTmuxForApp(t, body)

	dataDir := t.TempDir()
	bin := buildLazyTmuxBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin,
		"daemon",
		"--interval", "50ms",
		"--data-dir", dataDir,
		"--tmux-bin", tmuxBin,
	)
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(600 * time.Millisecond)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	logData, _ := os.ReadFile(logPath)

	path := filepath.Join(dataDir, "sessions", "demo.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(
			"expected snapshot to be created, got %v; log:\n%s\nTMUX log:\n%s",
			err,
			logBuf.String(),
			string(logData),
		)
	}
}

func buildLazyTmuxBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "lazy-tmux")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/lazy-tmux")
	cmd.Env = os.Environ()
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build lazy-tmux: %v\n%s", err, string(out))
	}
	return bin
}
