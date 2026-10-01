package tmux

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestAntexBindingHelper(t *testing.T) {
	t.Parallel()
	id := os.Getenv("LAZY_TMUX_BINDING_HELPER")
	if id == "" {
		return
	}
	socket, _, _ := strings.Cut(os.Getenv("TMUX"), ",")
	binding := antexBinding{
		Version:          1,
		ThreadID:         id,
		PID:              os.Getpid(),
		ProcessStartedAt: processStartTime(os.Getpid()),
		PaneID:           os.Getenv("TMUX_PANE"),
		SocketPath:       socket,
		Home:             "/tmp/antex-test",
		ResumeArgv:       []string{"antex", "resume", id, "--profile", "test profile"},
	}
	body, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		context.Background(),
		"tmux",
		"-S",
		socket,
		"set-option",
		"-p",
		"-t",
		binding.PaneID,
		"@antex_binding",
		string(body),
	)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("publish: %s %v", out, err)
	}
	time.Sleep(time.Minute)
}

func TestAntexBindingRoundTripWithRealTmux(t *testing.T) {
	t.Setenv("LAZY_TMUX_BINDING_HELPER", "")
	testutil.IsolatedTmux(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(t.TempDir(), "antex")
	if err := os.Symlink(executable, helper); err != nil {
		t.Fatal(err)
	}
	ids := []string{"01a0d3e1-d263-75c0-8189-c98bc7266f6c", "01a0cee1-2987-7b13-81cc-777ec4861be7"}
	for idx, id := range ids {
		command := "env LAZY_TMUX_BINDING_HELPER=" + id + " '" + helper + "' -test.run '^TestAntexBindingHelper$'"
		if idx == 0 {
			testutil.Tmux(t, "new-session", "-d", "-s", "binding", "-c", "/tmp", command)
		} else {
			testutil.Tmux(t, "new-window", "-t", "=binding:", "-c", "/tmp", command)
		}
	}
	client := NewClient("tmux")
	var captured snapshot.SessionSnapshot
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		captured, err = client.CaptureSession("binding")
		if err != nil {
			t.Fatal(err)
		}
		if len(captured.Windows) == 2 &&
			captured.Windows[0].Panes[0].Meta[snapshot.AntexSessionIDMetaKey] == ids[0] &&
			captured.Windows[1].Panes[0].Meta[snapshot.AntexSessionIDMetaKey] == ids[1] {
			break
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
			!strings.Contains(command, "'test profile'") {
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
	t.Setenv("LAZY_TMUX_BINDING_HELPER", "")
	testutil.IsolatedTmux(t)
	client := NewClient("tmux")
	client.SetRestoreTimeout(0)
	client.SetRestoreResolver(fakeResolver{matchCmd: "antex", override: "false"})
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
