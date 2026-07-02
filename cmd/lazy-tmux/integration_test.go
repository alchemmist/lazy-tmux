package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

// readSnapshot loads the on-disk JSON snapshot a command produced.
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

// TestSaveRestoreLifecycle drives the full save -> kill -> restore round trip
// against a real tmux server and asserts on real server state and the on-disk
// snapshot.
func TestSaveRestoreLifecycle(t *testing.T) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "work"

	// Build a real session: 2 windows, the first split into 2 panes.
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-n", "editor")
	testutil.Tmux(t, "split-window", "-d", "-t", "="+name+":0")
	testutil.Tmux(t, "new-window", "-d", "-t", "="+name+":", "-n", "shell")

	if got := windowCount(t, name); got != 2 {
		t.Fatalf("setup: expected 2 windows, got %d", got)
	}

	// Save.
	if code, _, errOut := run(t, "save", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("save: exit %d stderr=%q", code, errOut)
	}

	snap := readSnapshot(t, dir, name)
	if snap.SessionName != name || len(snap.Windows) != 2 {
		t.Fatalf("snapshot wrong: %+v", snap)
	}

	// First window should have captured 2 panes.
	if len(snap.Windows[0].Panes) != 2 {
		t.Fatalf("expected 2 panes in first window, got %d", len(snap.Windows[0].Panes))
	}

	// list shows it.
	if _, out, _ := run(t, "list", "--data-dir", dir); !strings.Contains(out, name) {
		t.Fatalf("list missing session: %q", out)
	}

	// Kill the live session, then restore from disk.
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

func TestWakeupAndSleep(t *testing.T) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "naps"

	testutil.Tmux(t, "new-session", "-d", "-s", name)

	// wakeup on a live session must fail (already awake).
	if code, _, errOut := run(t, "wakeup", "--session", name, "--data-dir", dir); code == 0 {
		t.Fatalf("wakeup on live session should fail, got exit 0 stderr=%q", errOut)
	}

	// sleep: save then kill.
	if code, _, errOut := run(t, "sleep", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("sleep: exit %d stderr=%q", code, errOut)
	}

	if sessionAlive(name) {
		t.Fatal("session should be asleep (killed) after sleep")
	}

	if _, err := os.Stat(filepath.Join(dir, "sessions", name+".json")); err != nil {
		t.Fatalf("sleep should have saved a snapshot: %v", err)
	}

	// wakeup on a saved-but-dead session restores it.
	if code, _, errOut := run(t, "wakeup", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("wakeup: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("session should be awake after wakeup")
	}
}

func TestSaveAllAndForget(t *testing.T) {
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

	// forget removes one.
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

// TestSaveAllNoSessionsReports covers issue #125: `save --all` must not be a
// silent no-op when there are no sessions (which is what happens when lazy-tmux
// is talking to a different/empty tmux than the user). It should say so.
func TestSaveAllNoSessionsReports(t *testing.T) {
	testutil.IsolatedTmux(t) // server is running but has no sessions

	dir := t.TempDir()

	code, out, errOut := run(t, "save", "--all", "--data-dir", dir)
	if code != 0 {
		t.Fatalf("save --all: exit %d stderr=%q", code, errOut)
	}

	if !strings.Contains(out, "no running tmux sessions found") {
		t.Fatalf("save --all with no sessions should report it, got %q", out)
	}
}

func TestBootstrapRestoresLatest(t *testing.T) {
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

func TestSaveWithScrollback(t *testing.T) {
	testutil.IsolatedTmux(t)

	dir := t.TempDir()

	const name = "logs"

	testutil.Tmux(t, "new-session", "-d", "-s", name)
	// Produce some shell output to capture as scrollback.
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

	// The snapshot must still be valid; scrollback capture is best-effort.
	snap := readSnapshot(t, dir, name)
	if snap.SessionName != name {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestPickerFZFEngineRestores(t *testing.T) {
	testutil.IsolatedTmux(t)
	testutil.RequireFZF(t)

	dir := t.TempDir()

	const name = "viafzf"

	testutil.Tmux(t, "new-session", "-d", "-s", name)

	if code, _, errOut := run(t, "save", "--session", name, "--data-dir", dir); code != 0 {
		t.Fatalf("save: exit %d stderr=%q", code, errOut)
	}

	testutil.Tmux(t, "kill-session", "-t", "="+name)

	// With no TTY, the fzf engine runs in --filter mode and selects the first
	// (here only) session, then restores it through real tmux.
	if code, _, errOut := run(t, "picker", "--fzf-engine", "--data-dir", dir); code != 0 {
		t.Fatalf("picker --fzf-engine: exit %d stderr=%q", code, errOut)
	}

	if !sessionAlive(name) {
		t.Fatal("picker should have restored the session via fzf engine")
	}
}
