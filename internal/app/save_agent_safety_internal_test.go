package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestIncompleteAgentSavePreservesSnapshotAndLiveSession(t *testing.T) {
	for _, kind := range []string{"pi", "claude", "codex", "antex"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SHELL", "/bin/sh")
			testutil.IsolatedTmux(t)
			a, dir := newTestApp(t)
			a.integrations = integration.NewRegistry(
				pi.New(dir, dir),
				claude.New(dir, ""),
				codex.New(dir),
				antex.New(dir),
			)
			original := snapshot.SessionSnapshot{
				SessionName: "kit",
				Windows: []snapshot.Window{
					{
						Index: 1,
						Name:  "webhooks",
						Panes: []snapshot.Pane{
							{
								Index:       1,
								CurrentCmd:  "pi",
								CurrentPath: dir,
								Meta: map[string]string{
									"pi.session_id":   "saved",
									"pi.session_file": "original.jsonl",
								},
							},
						},
					},
				},
			}
			if err := a.store.SaveSession(original); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "sessions", "kit.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			indexBefore, err := os.ReadFile(filepath.Join(dir, "index.json"))
			if err != nil {
				t.Fatal(err)
			}
			client := tmux.NewClient("tmux")
			client.SetRestoreResolver(a.integrations)
			client.SetRestoreTimeout(0)
			live := snapshot.SessionSnapshot{
				SessionName: "kit",
				Windows: []snapshot.Window{
					{
						Name:  "webhooks",
						Panes: []snapshot.Pane{{CurrentCmd: kind, CurrentPath: dir}},
					},
				},
			}
			if err := client.RestoreSession(t.Context(), live); err != nil {
				t.Fatal(err)
			}
			for _, save := range []func(string) error{a.SaveSession, a.Sleep, a.SaveSession} {
				err := save("kit")
				if err == nil || !strings.Contains(err.Error(), `window 0 ("webhooks") pane 0`) {
					t.Fatalf("missing actionable save error: %v", err)
				}
				if !a.tmux.SessionExists("kit") {
					t.Fatal("sleep killed a client with an incomplete capture")
				}
				assertAgentSaveFileUnchanged(t, path, before)
				assertAgentSaveFileUnchanged(t, filepath.Join(dir, "index.json"), indexBefore)
			}
		})
	}
}

func TestVerifiedPiCaptureCanSaveAndSleep(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires real Pi")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	a, dir := newTestApp(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "session.jsonl")
	header := `{"type":"session","version":3,"id":"exact-id","cwd":"` + filepath.ToSlash(
		dir,
	) + `"}` + "\n"
	if err := os.WriteFile(file, []byte(header), 0o600); err != nil {
		t.Fatal(err)
	}
	a.integrations = integration.NewRegistry(pi.New(home, dir))
	testutil.Tmux(
		t,
		"new-session",
		"-d",
		"-s",
		"kit",
		"-c",
		dir,
		"env",
		"PI_OFFLINE=1",
		"PI_CODING_AGENT_DIR="+home,
		"PI_CODING_AGENT_SESSION_DIR="+dir,
		"pi",
		"--no-tools",
		"--session",
		file,
	)
	for deadline := time.Now().Add(20 * time.Second); ; {
		raw := testutil.Tmux(
			t,
			"display-message",
			"-p",
			"-t",
			"=kit:0.0",
			"#{@lazy_tmux_pi_binding}",
		)
		if strings.Contains(raw, "exact-id") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no verified Pi binding: %s", raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := a.Sleep("kit"); err != nil {
		t.Fatal(err)
	}
	if a.tmux.SessionExists("kit") {
		t.Fatal("verified save did not permit sleep")
	}
	saved, err := a.store.LoadSession("kit")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Windows[0].Panes[0].Meta["pi.session_id"] != "exact-id" {
		t.Fatal("saved identity lost")
	}
}

func assertAgentSaveFileUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failed save changed %s", path)
	}
}
