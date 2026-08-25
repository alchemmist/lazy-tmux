package tmux_test

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

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

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreToleratesBaseIndexMismatch(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")

	const name = "legacy"

	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CurrentWin:  0,
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

	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatalf("restore with stale current_window must not fail: %v", err)
	}

	if _, err := testutil.TmuxTry("has-session", "-t", "="+name); err != nil {
		t.Fatalf("session should be alive after restore: %v", err)
	}

	if got := activeWindowIndex(t, name); got != "1" {
		t.Fatalf("expected window 1 focused, got %q", got)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreToleratesPaneBaseIndexMismatch(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

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

	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatalf("restore under base-index/pane-base-index 1 must not fail: %v", err)
	}

	if _, err := testutil.TmuxTry("has-session", "-t", "="+name); err != nil {
		t.Fatalf("session should be alive after restore: %v", err)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreWaitsForCommandsToStart(
	t *testing.T,
) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")

	const name = "settle"

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

	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	out := testutil.Tmux(t, "list-panes", "-t", "="+name, "-F", "#{pane_current_command}")
	if !strings.Contains(out, "cat") {
		t.Fatalf("expected pane already running cat right after restore, got %q", out)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreAllowlistFiltersCommands(
	t *testing.T,
) {
	assertGuardedRestore(t, "guarded", func(client *tmux.Client) {
		client.SetRestoreAllowlist([]string{"cat"})
	})
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreDenylistBlocksCommands(
	t *testing.T,
) {
	assertGuardedRestore(t, "denied", func(client *tmux.Client) {
		client.SetRestoreDenylist([]string{"tail"})
	})
}

func assertGuardedRestore(t *testing.T, name string, configure func(*tmux.Client)) {
	t.Helper()
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")
	configure(client)

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

	if err := client.RestoreSession(context.Background(), snap); err != nil {
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
		t.Fatalf("permitted pane should run cat, got %q (all: %v)", cmds["0"], cmds)
	}

	if cmds["1"] == "tail" {
		t.Fatalf("blocked pane must not run tail, but it did (all: %v)", cmds)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestRestoreHonorsRecordedFocus(
	t *testing.T,
) {
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

	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if got := activeWindowIndex(t, name); got != "2" {
		t.Fatalf("expected recorded window 2 focused, got %q", got)
	}
}

//nolint:paralleltest // uses a real shared tmux server via testutil.IsolatedTmux (t.Setenv)
func TestSynchronizeWindowSizeUsesAttachedClient(t *testing.T) {
	testutil.IsolatedTmux(t)

	client := tmux.NewClient("tmux")
	testutil.Tmux(t, "set-option", "-g", "window-size", "manual")
	testutil.Tmux(t, "new-session", "-d", "-s", "restored", "-x", "80", "-y", "24")

	ctx, cancel := context.WithCancel(context.Background())
	control := exec.CommandContext(ctx, "tmux", "-C", "attach-session", "-t", "=restored")
	control.Stdout = io.Discard
	control.Stderr = io.Discard
	stdin, err := control.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("control client stdin: %v", err)
	}
	if err = control.Start(); err != nil {
		cancel()
		t.Fatalf("start control client: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		cancel()
		_ = control.Wait()
	})

	clientName := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, listErr := testutil.TmuxTry(
			"list-clients",
			"-F",
			"#{client_name}|#{client_control_mode}|#{session_name}",
		)
		if listErr == nil {
			for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
				fields := strings.Split(line, "|")
				if len(fields) == 3 && fields[1] == "1" && fields[2] == "restored" {
					clientName = fields[0]

					break
				}
			}
		}
		if clientName != "" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if clientName == "" {
		t.Fatal("control client did not attach")
	}
	testutil.Tmux(t, "refresh-client", "-t", clientName, "-C", "180x56")

	before := strings.TrimSpace(
		testutil.Tmux(
			t,
			"display-message",
			"-p",
			"-t",
			"=restored:0.0",
			"#{pane_width}x#{pane_height}",
		),
	)
	if before != "80x24" {
		t.Fatalf("fixture must start undersized: got %s, want 80x24", before)
	}

	if err := client.SynchronizeWindowSize("restored:0"); err != nil {
		t.Fatalf("synchronize window size: %v", err)
	}

	after := strings.TrimSpace(
		testutil.Tmux(
			t,
			"display-message",
			"-p",
			"-t",
			"=restored:0.0",
			"#{pane_width}x#{pane_height}",
		),
	)
	if after != "180x56" {
		t.Fatalf("pane should fit attached client: got %s, want 180x56", after)
	}
}
