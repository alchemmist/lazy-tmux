package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type agentSaveTmux struct {
	tmuxClient

	state  snapshot.SessionSnapshot
	killed bool
}

func (m *agentSaveTmux) CaptureSession(
	string,
) (snapshot.SessionSnapshot, error) {
	return m.state, nil
}
func (m *agentSaveTmux) SessionExists(string) bool { return true }

func (m *agentSaveTmux) KillSession(
	string,
) error {
	m.killed = true

	return nil
}

func TestIncompleteAgentSavePreservesSnapshotAndLiveSession(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"pi", "claude", "codex", "antex"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
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
			m := &agentSaveTmux{
				state: snapshot.SessionSnapshot{
					SessionName: "kit",
					Windows: []snapshot.Window{
						{
							Index: 1,
							Name:  "webhooks",
							Panes: []snapshot.Pane{
								{Index: 1, CurrentCmd: kind, RestoreCmd: kind, CurrentPath: dir},
							},
						},
					},
				},
			}
			a.tmux = m
			for _, save := range []func(string) error{a.SaveSession, a.Sleep, a.SaveSession} {
				err := save("kit")
				if err == nil || !strings.Contains(err.Error(), `window 1 ("webhooks") pane 1`) {
					t.Fatalf("missing actionable save error: %v", err)
				}
				if m.killed {
					t.Fatal("sleep killed a client with an incomplete capture")
				}
				assertAgentSaveFileUnchanged(t, path, before)
				assertAgentSaveFileUnchanged(t, filepath.Join(dir, "index.json"), indexBefore)
			}
		})
	}
}

func TestVerifiedPiCaptureCanSaveAndSleep(t *testing.T) {
	t.Parallel()
	a, dir := newTestApp(t)
	file := filepath.Join(dir, "session.jsonl")
	header := `{"type":"session","id":"exact-id","cwd":"` + filepath.ToSlash(dir) + `"}` + "\n"
	if err := os.WriteFile(file, []byte(header), 0o600); err != nil {
		t.Fatal(err)
	}
	a.integrations = integration.NewRegistry(pi.New(dir, dir))
	m := &agentSaveTmux{
		state: snapshot.SessionSnapshot{
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
								"pi.session_file": file,
								"pi.session_id":   "exact-id",
							},
						},
					},
				},
			},
		},
	}
	a.tmux = m
	if err := a.Sleep("kit"); err != nil {
		t.Fatal(err)
	}
	if !m.killed {
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
