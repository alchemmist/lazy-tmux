package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

func TestRunSaveSessionSuccess(t *testing.T) {
	dataDir := t.TempDir()
	fake := writeFakeTmuxCLI(t, `
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

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"save",
		"--session", "demo",
		"--tmux-bin", fake,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}

	if _, err := store.New(dataDir).LoadSession("demo"); err != nil {
		t.Fatalf("expected saved snapshot, got %v", err)
	}
}

func TestRunSaveValidatesScrollbackLines(t *testing.T) {
	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{"save", "--scrollback", "--scrollback-lines", "0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	if !strings.Contains(errOut.String(), "save requires --scrollback-lines > 0") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunSaveAllSuccess(t *testing.T) {
	dataDir := t.TempDir()
	fake := writeFakeTmuxCLI(t, `
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

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"save",
		"--all",
		"--tmux-bin", fake,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}

	for _, name := range []string{"alpha", "beta"} {
		if _, err := store.New(dataDir).LoadSession(name); err != nil {
			t.Fatalf("expected %s snapshot saved, got %v", name, err)
		}
	}
}

func TestRunSaveCurrentSuccess(t *testing.T) {
	dataDir := t.TempDir()
	fake := writeFakeTmuxCLI(t, `
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

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"save",
		"--tmux-bin", fake,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}

	if _, err := store.New(dataDir).LoadSession("demo"); err != nil {
		t.Fatalf("expected saved snapshot, got %v", err)
	}
}

func TestRunRestoreSuccess(t *testing.T) {
	dataDir := t.TempDir()

	s := store.New(dataDir)
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 0, Name: "main", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "zsh"}}},
		},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	fake := writeFakeTmuxCLI(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
exit 0
`)

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"restore",
		"--session", "demo",
		"--switch=false",
		"--tmux-bin", fake,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}
}

func TestRunPickerFZFSuccess(t *testing.T) {
	dataDir := t.TempDir()

	s := store.New(dataDir)
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 0, Name: "main", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "zsh"}}},
		},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	fakeTmux := writeFakeTmuxCLI(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
exit 0
`)
	fakeFzfDir := t.TempDir()

	fakeFzf := filepath.Join(fakeFzfDir, "fzf")
	if err := os.WriteFile(fakeFzf, []byte("#!/bin/sh\nprintf 'demo\t2026-03-10 10:00:00\t1w\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake fzf: %v", err)
	}

	t.Setenv("PATH", fakeFzfDir+":"+os.Getenv("PATH"))

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"picker",
		"--fzf-engine",
		"--tmux-bin", fakeTmux,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}
}

func TestRunPickerRejectsBadSort(t *testing.T) {
	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{"picker", "--session-sort", "wat"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	if !strings.Contains(errOut.String(), "unknown session sort field") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunDaemonValidatesScrollbackLines(t *testing.T) {
	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{"daemon", "--scrollback", "--scrollback-lines", "0"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	if !strings.Contains(errOut.String(), "daemon requires --scrollback-lines > 0") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunSetupPrintsConfig(t *testing.T) {
	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{"setup"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	if !strings.Contains(out.String(), "lazy-tmux daemon") {
		t.Fatalf("unexpected setup output: %s", out.String())
	}
}

func TestRunBootstrapRestoresLastSession(t *testing.T) {
	dataDir := t.TempDir()

	s := store.New(dataDir)
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "demo",
		CapturedAt:  time.Now().UTC(),
		Windows: []snapshot.Window{
			{Index: 0, Name: "main", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", CurrentCmd: "zsh"}}},
		},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	fake := writeFakeTmuxCLI(t, `
if [ "$1" = "has-session" ]; then
  exit 1
fi
if [ "$1" = "list-windows" ]; then
  echo "0"
  exit 0
fi
exit 0
`)

	var out bytes.Buffer

	var errOut bytes.Buffer

	code := runCLI([]string{
		"bootstrap",
		"--session", "last",
		"--tmux-bin", fake,
		"--data-dir", dataDir,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}
}

func writeFakeTmuxCLI(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tmux")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	return path
}
