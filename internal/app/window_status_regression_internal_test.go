package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func startStatusPi(t *testing.T, name, dir string) (snapshot.Pane, string) {
	t.Helper()
	cwd := t.TempDir()
	file := filepath.Join(cwd, name+".jsonl")
	data, err := json.Marshal(
		map[string]any{"version": 3, "type": "session", "cwd": cwd, "id": name},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(
		testutil.Tmux(
			t,
			"new-session",
			"-d",
			"-P",
			"-F",
			"#{pane_id}",
			"-s",
			name,
			"-c",
			cwd,
			"env",
			"PI_OFFLINE=1",
			"PI_CODING_AGENT_DIR="+dir,
			"PI_CODING_AGENT_SESSION_DIR="+cwd,
			"pi",
			"--no-tools",
			"--session",
			file,
		),
	)
	client := tmux.NewClient("tmux")
	for deadline := time.Now().Add(20 * time.Second); ; {
		bound, err := client.CapturePane(pane)
		if err == nil && bound.LiveStatusID == name {
			bound.Meta["pi.session_id"] = name

			return bound, pane
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"Pi did not publish %q: %+v %v: %s",
				name,
				bound,
				err,
				testutil.Tmux(t, "show-options", "-pv", "-t", pane, "@lazy_tmux_pi_binding"),
			)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func setStatusInput(t *testing.T, pane, state string) {
	t.Helper()
	raw := testutil.Tmux(t, "show-options", "-pv", "-t", pane, "@lazy_tmux_pi_binding")
	var binding map[string]any
	if err := json.Unmarshal([]byte(raw), &binding); err != nil {
		t.Fatal(err)
	}
	binding["state"] = state
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(t, "set-option", "-p", "-t", pane, "@lazy_tmux_pi_binding", string(data))
}

func TestWindowStatusOrderingWithLiveBindings(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires real Pi")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	dir := t.TempDir()
	if _, err := setup.EnsurePi(dir); err != nil {
		t.Fatal(err)
	}
	reg := integration.NewRegistry(pi.New(dir, dir).WithStatusLoader(tmux.NewClient("tmux").CapturePiPanes)).
		Scope()
	statuses := []picker.WindowStatus{
		picker.StatusNone,
		picker.StatusIdle,
		picker.StatusAwaitingInput,
		picker.StatusWorking,
		picker.StatusNeedsSetup,
		picker.StatusError,
		picker.StatusAwaitingDecision,
		picker.StatusRestorePending,
	}
	panes := []snapshot.Pane{{CurrentCmd: "sh"}}
	for i, state := range []string{"idle", "awaiting_input", "working", "setup", "error", "awaiting_decision"} {
		if state == "setup" {
			panes = append(panes, snapshot.Pane{CurrentCmd: "claude"})

			continue
		}
		saved, pane := startStatusPi(t, fmt.Sprintf("pi-%d", i), dir)
		setStatusInput(t, pane, state)
		panes = append(panes, saved)
	}
	pending := panes[1]
	pending.Meta = maps.Clone(pending.Meta)
	pending.Meta["agent.restore_pending"] = "1"
	panes = append(panes, pending)
	for i, first := range panes {
		for j, second := range panes {
			got := windowStatuses(
				reg,
				[]snapshot.Window{
					{Index: 3, Panes: []snapshot.Pane{first, second}},
					{Index: 7, Panes: []snapshot.Pane{panes[1]}},
				},
			)
			if got[3] != statuses[max(i, j)] || got[7] != picker.StatusIdle {
				t.Fatalf("pair %d/%d: %v", i, j, got)
			}
		}
	}
	for _, key := range []string{"agent.restore_pending", "antex.restore_pending"} {
		for _, verified := range panes {
			waiting := verified
			waiting.Meta = make(map[string]string, len(verified.Meta)+1)
			maps.Copy(waiting.Meta, verified.Meta)
			waiting.Meta[key] = "1"
			if got := panePickerStatus(reg, waiting); got != picker.StatusRestorePending {
				t.Fatalf("%s on %s: %v", key, verified.CurrentCmd, got)
			}
		}
	}
	for offset := range panes {
		var rotated []snapshot.Pane
		rotated = append(rotated, panes[offset:]...)
		rotated = append(rotated, panes[:offset]...)
		if windowStatuses(reg, []snapshot.Window{{Index: 1, Panes: rotated}})[1] != picker.StatusRestorePending {
			t.Fatalf("rotation %d lost pending status", offset)
		}
	}
}

func TestWindowStatusesWithoutActivity(t *testing.T) {
	t.Parallel()
	windows := []snapshot.Window{{Index: 1, Panes: []snapshot.Pane{{CurrentCmd: "sh"}}}, {Index: 2}}
	if len(windowStatuses(nil, nil)) != 0 || windowStatuses(nil, windows)[1] != picker.StatusNone ||
		windowStatuses(integration.NewRegistry(), windows)[2] != picker.StatusNone ||
		toPickerStatus(integration.StatusUnknown) != picker.StatusNone {
		t.Fatal("empty inputs reported activity")
	}
}
