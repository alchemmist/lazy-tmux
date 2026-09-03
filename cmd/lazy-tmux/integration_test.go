package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestConcurrentSaveAllPreservesEverySession(t *testing.T) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()
	const sessionCount = 8
	for index := range sessionCount {
		name := fmt.Sprintf("concurrent-%d", index)
		testutil.Tmux(t, "new-session", "-d", "-s", name)
		testutil.Tmux(t, "new-window", "-d", "-t", "="+name+":", "-n", "extra")
	}

	type result struct {
		code   int
		stderr string
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			code, _, stderr := run(t, "save", "--all", "--data-dir", dir)
			results <- result{code: code, stderr: stderr}
		})
	}
	group.Wait()
	close(results)
	for result := range results {
		if result.code != 0 {
			t.Fatalf("concurrent save --all: exit %d stderr=%q", result.code, result.stderr)
		}
	}

	for index := range sessionCount {
		name := fmt.Sprintf("concurrent-%d", index)
		snap := readSnapshot(t, dir, name)
		if len(snap.Windows) != 2 {
			t.Fatalf("%s has %d windows, want 2", name, len(snap.Windows))
		}
	}
}

func readSnapshot(t *testing.T, dir, name string) snapshot.SessionSnapshot {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(dir, "sessions", name+".json"))
	if err != nil {
		t.Fatalf("read snapshot %s: %v", name, err)
	}

	var snap snapshot.SessionSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatalf("unmarshal snapshot %s: %v", name, err)
	}

	return snap
}

func sessionAlive(name string) bool {
	_, err := testutil.TmuxTry("has-session", "-t", "="+name)

	return err == nil
}

func windowCount(t *testing.T, name string) int {
	t.Helper()

	out := testutil.Tmux(t, "list-windows", "-t", "="+name, "-F", "#{window_index}")

	count := 0

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return count
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSaveRestoreLifecycle(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "work"

	testutil.Tmux(t, "new-session", "-d", "-s", name, "-n", "editor")
	testutil.Tmux(t, "split-window", "-d", "-t", "="+name+":0")
	testutil.Tmux(t, "new-window", "-d", "-t", "="+name+":", "-n", "shell")

	if got := windowCount(t, name); got != 2 {
		t.Fatalf("setup: expected 2 windows, got %d", got)
	}

	if code, _, errOut := run(t, "save", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("save: exit %d stderr=%q", code, errOut)
	}

	snap := readSnapshot(t, dir, name)
	if snap.SessionName != name || len(snap.Windows) != 2 {
		t.Fatalf("snapshot wrong: %+v", snap)
	}

	if len(snap.Windows[0].Panes) != 2 {
		t.Fatalf("expected 2 panes in first window, got %d", len(snap.Windows[0].Panes))
	}

	if _, out, _ := run(t, "list", "--data-dir", dir); !strings.Contains(out, name) {
		t.Fatalf("list missing session: %q", out)
	}

	testutil.Tmux(t, "kill-session", "-t", "="+name)

	if sessionAlive(name) {
		t.Fatal("session should be dead before restore")
	}

	if code, _, errOut := run(
		t,
		"restore",
		"--session",
		name,
		"--switch=false",
		"--data-dir",
		dir,
	); code != 0 {
		t.Fatalf("restore: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("session should be alive after restore")
	}

	if got := windowCount(t, name); got != 2 {
		t.Fatalf("after restore expected 2 windows, got %d", got)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestWakeupAndSleep(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "naps"

	testutil.Tmux(t, "new-session", "-d", "-s", name)

	if code, _, errOut := run(t, "wakeup", "--session", name, "--data-dir", dir); code == 0 {
		t.Fatalf("wakeup on live session should fail, got exit 0 stderr=%q", errOut)
	}

	if code, _, errOut := run(t, "sleep", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("sleep: exit %d stderr=%q", code, errOut)
	}

	if sessionAlive(name) {
		t.Fatal("session should be asleep (killed) after sleep")
	}

	if _, err := os.Stat(filepath.Join(dir, "sessions", name+".json")); err != nil {
		t.Fatalf("sleep should have saved a snapshot: %v", err)
	}

	if code, _, errOut := run(t, "wakeup", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("wakeup: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("session should be awake after wakeup")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSaveAllAndForget(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	testutil.Tmux(t, "new-session", "-d", "-s", "one")
	testutil.Tmux(t, "new-session", "-d", "-s", "two")

	if code, out, errOut := run(t, "save", "--all", "--data-dir", dir); code != 0 {
		t.Fatalf("save --all: exit %d stderr=%q", code, errOut)
	} else if !strings.Contains(out, "saved 2 session(s)") {
		t.Fatalf("save --all should report the saved count, got %q", out)
	}

	for _, n := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(dir, "sessions", n+".json")); err != nil {
			t.Fatalf("save --all missing %s: %v", n, err)
		}
	}

	if code, _, errOut := run(t, "forget", "--session", "one", "--data-dir", dir); code != 0 {
		t.Fatalf("forget: exit %d stderr=%q", code, errOut)
	}

	if _, err := os.Stat(filepath.Join(dir, "sessions", "one.json")); !os.IsNotExist(err) {
		t.Fatalf("forget should remove snapshot, stat err=%v", err)
	}

	_, out, _ := run(t, "list", "--data-dir", dir)
	if strings.Contains(out, "one\t") {
		t.Fatalf("forgotten session still listed: %q", out)
	}

	if !strings.Contains(out, "two") {
		t.Fatalf("remaining session missing from list: %q", out)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSaveAllNoSessionsReports(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	code, out, errOut := run(t, "save", "--all", "--data-dir", dir)
	if code != 0 {
		t.Fatalf("save --all: exit %d stderr=%q", code, errOut)
	}

	if !strings.Contains(out, "no running tmux sessions found") {
		t.Fatalf("save --all with no sessions should report it, got %q", out)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestBootstrapRestoresLatest(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "boot"

	testutil.Tmux(t, "new-session", "-d", "-s", name)

	if code, _, errOut := run(t, "save", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("save: exit %d stderr=%q", code, errOut)
	}

	testutil.Tmux(t, "kill-session", "-t", "="+name)

	if code, _, errOut := run(t, "bootstrap", "--session", "last", "--data-dir", dir); code != 0 {
		t.Fatalf("bootstrap: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("bootstrap last should have restored the saved session")
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSaveWithScrollback(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "logs"

	testutil.Tmux(t, "new-session", "-d", "-s", name)
	testutil.Tmux(t, "send-keys", "-t", name+":0.0", "echo lazytmuxscrollback", "C-m")

	if code, _, errOut := run(
		t,
		"save",
		"--session",
		name,
		"--scrollback",
		"--data-dir",
		dir,
	); code != 0 {
		t.Fatalf("save --scrollback: exit %d stderr=%q", code, errOut)
	}

	snap := readSnapshot(t, dir, name)
	if snap.SessionName != name {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestPickerFZFEngineRestores(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)
	testutil.RequireFZF(t)

	dir := t.TempDir()

	const name = "viafzf"

	testutil.Tmux(t, "new-session", "-d", "-s", name)

	if code, _, errOut := run(t, "save", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("save: exit %d stderr=%q", code, errOut)
	}

	testutil.Tmux(t, "kill-session", "-t", "="+name)

	if code, _, errOut := run(t, "picker", "--fzf-engine", "--data-dir", dir); code != 0 {
		t.Fatalf("picker --fzf-engine: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("picker should have restored the session via fzf engine")
	}
}
