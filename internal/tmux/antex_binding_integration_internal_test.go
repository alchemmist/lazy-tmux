package tmux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestAntexBindingRoundTripWithRealTmux(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	home, cwd := t.TempDir(), t.TempDir()
	first, _ := testutil.StartAntex(t, home, cwd, "new-session", "-s", "binding")
	second, _ := testutil.StartAntex(t, home, cwd, "new-window", "-t", "=binding:")
	ids := []string{first, second}
	if ids[0] == ids[1] {
		t.Fatal("independent clients published the same identity")
	}
	client := NewClient("tmux")
	var err error
	var captured snapshot.SessionSnapshot
	deadline := time.Now().Add(10 * time.Second)
	for {
		captured, err = client.CaptureSession("binding")
		if err != nil {
			t.Fatal(err)
		}
		if len(captured.Windows) == 2 && len(captured.Windows[0].Panes) > 0 &&
			len(captured.Windows[1].Panes) > 0 &&
			captured.Windows[0].Panes[0].Meta[snapshot.AntexSessionIDMetaKey] == ids[0] &&
			captured.Windows[1].Panes[0].Meta[snapshot.AntexSessionIDMetaKey] == ids[1] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out capturing Antex owners %v: %+v", ids, captured.Windows)
		}
		time.Sleep(20 * time.Millisecond)
	}
	registry := integration.NewRegistry(antex.New(t.TempDir()))
	registry.Enrich(&captured)
	storage := store.New(t.TempDir())
	if err := storage.SaveSession(captured); err != nil {
		t.Fatal(err)
	}
	saved, err := storage.LoadSession("binding")
	if err != nil {
		t.Fatal(err)
	}
	for idx, id := range ids {
		pane := saved.Windows[idx].Panes[0]
		if pane.Meta[snapshot.AntexSessionIDMetaKey] != id {
			t.Fatalf("window %d identity: %v", idx, pane.Meta)
		}
		command, err := registry.ResolveChecked(pane)
		if err != nil || !strings.Contains(command, "'resume' '"+id+"'") ||
			!strings.Contains(command, `model_provider="local"`) {
			t.Fatalf("window %d restore: %q %v", idx, command, err)
		}
	}
	testutil.Tmux(t, "respawn-pane", "-k", "-t", "=binding:0.0", "sleep 60")
	stale, err := client.CapturePane("=binding:0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(stale.Meta) != 0 {
		t.Fatalf("dead owner binding survived respawn: %v", stale.Meta)
	}
}

func TestFailedBootstrapSurvivesRealTmuxCapture(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	testutil.IsolatedTmux(t)
	client := NewClient("tmux")
	client.SetRestoreTimeout(0)
	client.SetRestoreResolver(integration.NewRegistry(antex.New(t.TempDir())))
	client.SetRestoreAllowlist([]string{"env .*'antex' 'resume' .*"})
	testutil.Tmux(t, "set-environment", "-g", "PATH", "/nonexistent")
	id := "01a09f96-a7d1-75f1-b8d8-6df25aaa7e6c"
	original := snapshot.SessionSnapshot{
		SessionName: "failed-bootstrap",
		Windows: []snapshot.Window{
			{
				Panes: []snapshot.Pane{
					{
						CurrentCmd:  "antex",
						CurrentPath: "/tmp",
						Meta: map[string]string{
							snapshot.AntexSessionIDMetaKey: id,
							"antex.session_id_source":      "binding-v1",
							"antex.home":                   t.TempDir(),
							"antex.resume_argv":            `["antex","resume","` + id + `"]`,
						},
					},
				},
			},
		},
	}
	if err := client.RestoreSession(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	got, err := client.CaptureSession("failed-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	pane := got.Windows[0].Panes[0]
	if pane.CurrentCmd != "antex" || pane.Meta[snapshot.AntexSessionIDMetaKey] != id ||
		pane.Meta[pendingRestoreMeta] != "1" {
		t.Fatalf("failed launch erased resume identity: %+v", pane)
	}
	storage := store.New(t.TempDir())
	if err := storage.SaveSession(got); err != nil {
		t.Fatal(err)
	}
	saved, err := storage.LoadSession("failed-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Windows[0].Panes[0].Meta[snapshot.AntexSessionIDMetaKey] != id {
		t.Fatal("autosave lost original identity")
	}
}
