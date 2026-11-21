//go:build !lazy_fzf

package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestPickerPTYChild(t *testing.T) {
	t.Parallel()
	args := flag.Args()
	if len(args) != 2 || args[0] != "picker-pty" {
		t.Skip("subprocess entry point")
	}
	a := New(config.Config{TmuxBin: "tmux", DataDir: args[1]})
	target, err := a.SelectTargetWithTUI()
	var result string
	switch {
	case err != nil:
		result = err.Error()
	case target.WindowIndex != nil:
		result = fmt.Sprintf("%s:%d", target.SessionName, *target.WindowIndex)
	default:
		result = target.SessionName
	}
	if err := os.WriteFile(filepath.Join(args[1], "selection"), []byte(result), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitPickerScreen(t *testing.T, pane, text string) string {
	t.Helper()
	for deadline := time.Now().Add(8 * time.Second); ; {
		screen := testutil.Tmux(t, "capture-pane", "-pJ", "-t", pane)
		if strings.Contains(screen, text) {
			return screen
		}
		if time.Now().After(deadline) {
			t.Fatalf("screen never contained %q:\n%s", text, screen)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func startPickerPTY(t *testing.T, dir string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(
		testutil.Tmux(
			t,
			"new-session",
			"-d",
			"-P",
			"-F",
			"#{pane_id}",
			"-x",
			"110",
			"-y",
			"30",
			exe,
			"-test.run=^TestPickerPTYChild$",
			"--",
			"picker-pty",
			dir,
		),
	)
}

func TestPickerPTYLoadsSelectsAndCancels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	a, dir := newTestApp(t)
	if err := a.store.SaveSession(
		snapshot.SessionSnapshot{
			SessionName: "alpha",
			Windows: []snapshot.Window{
				{Index: 2, Name: "details-ready", Panes: []snapshot.Pane{{CurrentCmd: "sh"}}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	lock, err := os.Open(filepath.Join(dir, ".session-locks", "alpha.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	pane := startPickerPTY(t, dir)
	if screen := waitPickerScreen(t, pane, "alpha"); strings.Contains(screen, "details-ready") {
		t.Fatal("locked details rendered before release")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, want string }{{"M-2", "alpha:2"}, {"Escape", "selection canceled"}} {
		if tc.key == "Escape" {
			pane = startPickerPTY(t, dir)
		}
		waitPickerScreen(t, pane, "details-ready")
		testutil.Tmux(t, "send-keys", "-t", pane, tc.key)
		path := filepath.Join(dir, "selection")
		for deadline := time.Now().Add(8 * time.Second); ; {
			body, err := os.ReadFile(path)
			if err == nil && strings.Contains(string(body), tc.want) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("result %q (%v), want %q", body, err, tc.want)
			}
			time.Sleep(25 * time.Millisecond)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

type actionScenario struct {
	name    string
	keys    []string
	target  string
	windows string
}

func TestPickerPTYActions(t *testing.T) {
	for _, tc := range []actionScenario{
		{"delete window", []string{"C-d", "Enter"}, "alpha", "two,three"},
		{"delete session", []string{"M-d", "Enter"}, "alpha", "absent"},
		{"delete marked windows", []string{"C-d", "C-j", "Space", "Enter"}, "alpha", "three"},
		{"rename window", []string{"C-r", "Enter", "X", "Enter"}, "alpha", "oneX,two,three"},
		{"rename session", []string{"M-r", "Enter", "Z", "Enter"}, "alphaZ", "one,two,three"},
		{"new session", []string{"M-n", "Enter", "qa", "Enter"}, "qa", "live"},
		{"new window", []string{"C-n", "Enter", "Enter"}, "alpha", "one,two,three,window-4"},
		{"filtered new window", []string{"M-n", "b", "Enter", "Enter"}, "beta", "bwin,window-2"},
		{"theme", []string{"/theme light", "Enter"}, "alpha", "theme"},
		{"wake", []string{"M-w", "Enter"}, "alpha", "live"},
		{"sleep", []string{"M-s", "Enter"}, "alpha", "asleep"},
		{"delete error", []string{"C-d", "Enter"}, "alpha", "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SHELL", "/bin/sh")
			t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
			testutil.IsolatedTmux(t)
			a, dir := newTestApp(t)
			seedPickerAction(t, a, tc)
			pane := startPickerPTY(t, dir)
			waitPickerScreen(t, pane, "three")
			if tc.windows == "error" {
				path, err := a.store.SessionPath("alpha")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			testutil.Tmux(t, append([]string{"send-keys", "-t", pane}, tc.keys...)...)
			if tc.windows == "error" {
				waitPickerScreen(t, pane, "load session")

				return
			}
			waitPickerMutation(t, a, pane, tc)
		})
	}
}

func waitPickerMutation(t *testing.T, a *App, pane string, tc actionScenario) {
	t.Helper()
	for deadline := time.Now().Add(6 * time.Second); ; {
		state, err := a.store.LoadSession(tc.target)
		names := make([]string, 0, len(state.Windows))
		for _, w := range state.Windows {
			names = append(names, w.Name)
		}
		switch tc.windows {
		case "theme":
			cfg, err := config.Load()
			if err == nil && cfg.Theme == "light" {
				return
			}
		case "absent":
			if errors.Is(err, os.ErrNotExist) {
				return
			}
		case "live":
			if err == nil && a.tmux.SessionExists(tc.target) {
				return
			}
		case "asleep":
			if err == nil && !a.tmux.SessionExists(tc.target) {
				return
			}
		default:
			if err == nil && strings.Join(names, ",") == tc.windows {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"%s: windows %v, error %v; screen:\n%s",
				tc.name,
				names,
				err,
				testutil.Tmux(t, "capture-pane", "-pJ", "-t", pane),
			)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func seedPickerAction(t *testing.T, a *App, tc actionScenario) {
	t.Helper()
	windows := make([]snapshot.Window, 0, 3)
	for i, name := range []string{"one", "two", "three"} {
		windows = append(
			windows,
			snapshot.Window{
				Index: i + 1,
				Name:  name,
				Panes: []snapshot.Pane{{CurrentCmd: "sh"}},
			},
		)
	}
	if err := a.store.SaveSession(
		snapshot.SessionSnapshot{SessionName: "alpha", Windows: windows},
	); err != nil {
		t.Fatal(err)
	}
	if tc.target == "beta" {
		if err := a.store.SaveSession(
			snapshot.SessionSnapshot{
				SessionName: "beta",
				Windows: []snapshot.Window{
					{Index: 1, Name: "bwin", Panes: []snapshot.Pane{{CurrentCmd: "sh"}}},
				},
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	if tc.windows == "asleep" {
		if err := a.Wakeup("alpha"); err != nil {
			t.Fatal(err)
		}
	}
}
