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
