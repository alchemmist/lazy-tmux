package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeFakeFZF(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf")
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fzf: %v", err)
	}
	return dir
}

func TestChooseSessionSuccess(t *testing.T) {
	stdinLog := filepath.Join(t.TempDir(), "stdin.log")
	fzfDir := writeFakeFZF(t, `
cat > "$FZF_STDIN_LOG"
printf 'beta\t2026-01-01 12:00:00\t2w/3p\n'
`)
	t.Setenv("FZF_STDIN_LOG", stdinLog)
	t.Setenv("PATH", fzfDir+":"+os.Getenv("PATH"))

	recs := []snapshot.Record{
		{SessionName: "alpha", CapturedAt: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC), Windows: 1, Panes: 1},
		{SessionName: "beta", CapturedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), Windows: 2, Panes: 3},
	}

	got, err := chooseSession(recs)
	if err != nil {
		t.Fatalf("chooseSession error: %v", err)
	}
	if got != "beta" {
		t.Fatalf("expected beta, got %q", got)
	}

	b, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("read stdin log: %v", err)
	}
	in := string(b)
	if !strings.Contains(in, "alpha\t") || !strings.Contains(in, "beta\t") {
		t.Fatalf("expected both sessions in fzf input, got:\n%s", in)
	}
}

func TestChooseSessionEmptySelection(t *testing.T) {
	fzfDir := writeFakeFZF(t, "exit 0")
	t.Setenv("PATH", fzfDir+":"+os.Getenv("PATH"))

	_, err := chooseSession([]snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC()}})
	if err == nil {
		t.Fatal("expected error for empty selection")
	}
	if !strings.Contains(err.Error(), "no session selected") {
		t.Fatalf("unexpected error: %v", err)
	}
}
