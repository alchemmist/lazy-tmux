package tmux_test

import (
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

// activeWindowIndex returns the index of the active window in a live session,
// reading it straight from the isolated tmux server.
func activeWindowIndex(t *testing.T, name string) string {
	t.Helper()

	out := testutil.Tmux(
		t,
		"list-windows",
		"-t",
		"="+name,
		"-F",
		"#{window_index} #{window_active}",
	)
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "1" {
			return fields[0]
		}
	}

	t.Fatalf("no active window found in %q", out)

	return ""
}

// TestRestoreToleratesBaseIndexMismatch reproduces issues #103/#105: a snapshot
// captured under `set -g base-index 1` records windows indexed from 1 while the
// current window is stored as 0. Restoring it used to fail hard with
// "can't find window: 0" because the focus selection trusted the recorded index
// blindly. The restore must now succeed and focus the only real window instead.
func TestRestoreToleratesBaseIndexMismatch(t *testing.T) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")

	const name = "legacy"

	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  0, // stale: no window has index 0
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index:      1,
				Name:       "main",
				IsActive:   true,
				ActivePane: 0,
				Panes:      []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", IsActive: true}},
			},
		},
	}

	if err := client.RestoreSession(snap); err != nil {
		t.Fatalf("restore with stale current_window must not fail: %v", err)
	}

	if _, err := testutil.TmuxTry("has-session", "-t", "="+name); err != nil {
		t.Fatalf("session should be alive after restore: %v", err)
	}

	if got := activeWindowIndex(t, name); got != "1" {
		t.Fatalf("expected window 1 focused, got %q", got)
	}
}

// TestRestoreToleratesPaneBaseIndexMismatch covers the `bootstrap` crash where a
// snapshot captured under base-index/pane-base-index 0 is restored on a server
// configured with base-index 1 and pane-base-index 1 (a common user config). The
// recorded window 0 / pane 0 need not exist after restore, so focusing them used
// to fail hard ("can't find window: 0" / "can't find pane: 0") and abort the
// whole restore. The restore must now succeed and leave the session alive.
func TestRestoreToleratesPaneBaseIndexMismatch(t *testing.T) {
	testutil.IsolatedTmux(t)

	// Reconfigure the isolated server to index windows and panes from 1, so the
	// snapshot's 0-based indices won't all map onto restored objects.
	testutil.Tmux(t, "set-option", "-g", "base-index", "1")
	testutil.Tmux(t, "set-option", "-g", "pane-base-index", "1")

	client := tmux.NewClient("tmux")

	const name = "home"

	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 0, Name: "w0", IsActive: true, ActivePane: 0,
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", IsActive: true}},
			},
			{
				Index: 1, Name: "w1", ActivePane: 0,
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", IsActive: true}},
			},
		},
	}

	if err := client.RestoreSession(snap); err != nil {
		t.Fatalf("restore under base-index/pane-base-index 1 must not fail: %v", err)
	}

	if _, err := testutil.TmuxTry("has-session", "-t", "="+name); err != nil {
		t.Fatalf("session should be alive after restore: %v", err)
	}
}

// TestRestoreWaitsForCommandsToStart is the end-to-end counterpart to issue
// #106: against a real tmux server, once RestoreSession returns the pane must
// actually be running its restored command rather than sitting at the shell.
// (The fine-grained "did it really block?" guarantee is covered deterministically
// by TestWaitForRestoredCommands* with a fake runner; here we confirm the whole
// real-tmux path settles correctly.)
func TestRestoreWaitsForCommandsToStart(t *testing.T) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")

	const name = "settle"

	// `cat` with no args blocks reading stdin, so it stays in the foreground
	// long enough to observe as pane_current_command.
	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  1,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 1, Name: "main", IsActive: true, ActivePane: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/tmp", RestoreCmd: "cat", IsActive: true},
				},
			},
		},
	}

	if err := client.RestoreSession(snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// No sleep here on purpose: if the restore returned before the command
	// started, the pane would still be running the shell.
	out := testutil.Tmux(t, "list-panes", "-t", "="+name, "-F", "#{pane_current_command}")
	if !strings.Contains(out, "cat") {
		t.Fatalf("expected pane already running cat right after restore, got %q", out)
	}
}

// TestRestoreAllowlistFiltersCommands covers issue #74 end-to-end: with an
// allowlist configured, only permitted commands are replayed; a disallowed
// command leaves its pane at the shell.
func TestRestoreAllowlistFiltersCommands(t *testing.T) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")
	client.SetRestoreAllowlist([]string{"cat"}) // allow cat, block everything else

	const name = "guarded"

	// Two panes: one runs an allowed command (cat), one a blocked command (tail).
	// Both block on stdin, so if restored they would show as the foreground cmd.
	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  1,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 1, Name: "main", IsActive: true, ActivePane: 0,
				Panes: []snapshot.Pane{
					{Index: 0, CurrentPath: "/tmp", RestoreCmd: "cat", IsActive: true},
					{Index: 1, CurrentPath: "/tmp", RestoreCmd: "tail"},
				},
			},
		},
	}

	if err := client.RestoreSession(snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	out := testutil.Tmux(
		t,
		"list-panes",
		"-t",
		"="+name,
		"-F",
		"#{pane_index} #{pane_current_command}",
	)

	cmds := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 {
			cmds[fields[0]] = fields[1]
		}
	}

	if cmds["0"] != "cat" {
		t.Fatalf("allowed pane should run cat, got %q (all: %v)", cmds["0"], cmds)
	}

	if cmds["1"] == "tail" {
		t.Fatalf("blocked pane must not run tail, but it did (all: %v)", cmds)
	}
}

// TestRestoreHonorsRecordedFocus guards the happy path: when the recorded
// current window exists, restore focuses exactly that window rather than
// falling back.
func TestRestoreHonorsRecordedFocus(t *testing.T) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")

	const name = "multi"

	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  2,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{
				Index: 1, Name: "one", ActivePane: 0,
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp"}},
			},
			{
				Index: 2, Name: "two", IsActive: true, ActivePane: 0,
				Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp", IsActive: true}},
			},
		},
	}

	if err := client.RestoreSession(snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if got := activeWindowIndex(t, name); got != "2" {
		t.Fatalf("expected recorded window 2 focused, got %q", got)
	}
}
