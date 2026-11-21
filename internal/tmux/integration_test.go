package tmux_test

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
func TestCaptureSessionCollectsAllWindowsAndPanes(t *testing.T) {
	testutil.IsolatedTmux(t)

	const name = "capture-many"
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-n", "one")
	testutil.Tmux(t, "split-window", "-d", "-t", "="+name+":0", "-c", "/tmp")
	testutil.Tmux(t, "new-window", "-d", "-t", "="+name+":2", "-n", "two", "-c", "/")
	testutil.Tmux(t, "set-option", "-p", "-t", "="+name+":2.0", "@antex_thread_id", "thread-2")
	testutil.Tmux(t, "select-window", "-t", "="+name+":2")

	snap, err := tmux.NewClient("tmux").CaptureSession(name)
	if err != nil {
		t.Fatalf("capture session: %v", err)
	}
	if len(snap.Windows) != 2 || len(snap.Windows[0].Panes) != 2 ||
		len(snap.Windows[1].Panes) != 1 {
		t.Fatalf("captured hierarchy: %+v", snap.Windows)
	}
	if snap.CurrentWin != 2 || snap.Windows[1].Name != "two" {
		t.Fatalf("captured focus/windows: %+v", snap)
	}
	if got := snap.Windows[1].Panes[0].Meta[snapshot.AntexSessionIDMetaKey]; got != "" {
		t.Fatalf("unowned legacy Antex thread should not be trusted: %q", got)
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
func TestRestoreStopsAfterRealContextCancellation(t *testing.T) {
	testutil.IsolatedTmux(t)

	const name = "cancel-real"
	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CapturedAt:  time.Now(),
		CurrentWin:  0,
		CurrentPane: 0,
		Windows: []snapshot.Window{
			{Index: 0, Name: "first", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp"}}},
			{Index: 1, Name: "second", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp"}}},
			{Index: 2, Name: "third", Panes: []snapshot.Pane{{Index: 0, CurrentPath: "/tmp"}}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := tmux.NewClient("tmux").RestoreSession(ctx, snap)
	if err == nil || !strings.Contains(err.Error(), "restore canceled") {
		t.Fatalf("restore cancellation error = %v", err)
	}
	out := testutil.Tmux(t, "list-windows", "-t", "="+name, "-F", "#{window_index}")
	if got := len(strings.Fields(out)); got != 1 {
		t.Fatalf("canceled restore created %d windows, want only initial window", got)
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
	testutil.Tmux(t, "new-session", "-d", "-s", "restored", "-x", "80", "-y", "24")
	testutil.Tmux(t, "set-option", "-w", "-t", "=restored:0", "window-size", "manual")

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
			"#{client_name}|#{session_name}",
		)
		if listErr == nil {
			for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
				fields := strings.Split(line, "|")
				if len(fields) == 2 && fields[1] == "restored" {
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
	if _, err = testutil.TmuxTry("refresh-client", "-t", clientName, "-C", "180x56"); err != nil {
		testutil.Tmux(t, "refresh-client", "-t", clientName, "-C", "180,56")
	}

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
	if before != "80x23" && before != "80x24" {
		t.Fatalf("fixture must start undersized: got %s, want 80x23 or 80x24", before)
	}

	if err := client.SynchronizeWindowSize("restored:0"); err != nil {
		t.Fatalf("synchronize window size: %v", err)
	}

	local := testutil.Tmux(t, "show-options", "-w", "-t", "=restored:0", "window-size")
	if strings.TrimSpace(local) != "" {
		t.Fatalf("window-size override was not cleared: %s", local)
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
	if after != "180x55" && after != "180x56" {
		t.Fatalf("pane should fit attached client: got %s, want 180x55 or 180x56", after)
	}
}

func TestAttachSessionChild(t *testing.T) {
	t.Parallel()
	args := flag.Args()
	if len(args) != 2 || args[0] != "attach-child" {
		t.Skip("subprocess entry point")
	}
	client := tmux.NewClient("tmux")
	if args[1] == "proj:2" {
		err := client.AttachSession("invalid\x00target")
		if !errors.Is(err, syscall.EINVAL) || !strings.Contains(err.Error(), "attach tmux client") {
			t.Fatalf("attach with invalid argv: got %v, want wrapped EINVAL", err)
		}
	}
	if err := client.AttachSession(args[1]); err != nil {
		t.Fatal(err)
	}
	t.Log("attach returned without exec")
}

func isolateAttachXDG(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	} {
		t.Setenv(key, t.TempDir())
	}
}

func TestAttachSessionWithoutTTY(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	isolateAttachXDG(t)
	testutil.IsolatedTmux(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "proj")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		exe,
		"-test.run=^TestAttachSessionChild$",
		"-test.v",
		"--",
		"attach-child",
		"proj",
	)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "attach returned without exec") {
		t.Fatalf("noninteractive attach: %v: %s", err, out)
	}
}

func TestAttachSessionWithTTY(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	isolateAttachXDG(t)
	testutil.IsolatedTmux(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(t, "new-session", "-d", "-s", "proj")
	testutil.Tmux(t, "new-window", "-d", "-t", "proj:2")
	testutil.Tmux(
		t,
		"new-session",
		"-d",
		"-s",
		"driver",
		"env",
		"-u",
		"TMUX",
		exe,
		"-test.run=^TestAttachSessionChild$", "--", "attach-child", "proj:2",
	)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		out := testutil.Tmux(t, "list-clients", "-F", "#{session_name}:#{window_index}")
		if strings.TrimSpace(out) == "proj:2" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("client did not attach to proj:2")
}
