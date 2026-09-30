package tmux

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestAgentHookHelper(t *testing.T) {
	t.Parallel()
	kind := os.Getenv("LAZY_TEST_AGENT")
	if kind == "" {
		return
	}
	if os.Getenv("LAZY_TEST_HOOK") == "1" {
		event := AgentHookEvent{
			SessionID: os.Getenv("LAZY_TEST_ID"),
			CWD:       "/tmp",
			Event:     "SessionStart",
		}
		if err := NewClient(
			"tmux",
		).ApplyAgentHook(kind, "/tmp/agent-home", os.Getenv("TMUX_PANE"), event); err != nil {
			t.Fatal(err)
		}

		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(
		context.Background(),
		executable,
		"-test.run",
		"^TestAgentHookHelper$",
	)
	command.Env = append(os.Environ(), "LAZY_TEST_HOOK=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %s %v", output, err)
	}
	time.Sleep(time.Minute)
}

func TestThreeAgentHooksRoundTripWithoutAccounts(t *testing.T) {
	t.Setenv("LAZY_TEST_AGENT", "")
	testutil.IsolatedTmux(t)
	testutil.Tmux(t, "set-option", "-g", "remain-on-exit", "on")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"codex", "claude"} {
		helper := filepath.Join(t.TempDir(), kind)
		if err := os.Symlink(executable, helper); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"first", "second"} {
			name := kind + "-" + id
			command := "env LAZY_TEST_AGENT=" + kind + " LAZY_TEST_ID=" + id + " '" + helper + "'"
			testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", "/tmp", command)
		}
	}
	registry := integration.NewRegistry(
		codex.New("/tmp/agent-home"),
		claude.New("/tmp/agent-home", t.TempDir()),
	)
	for _, kind := range []string{"codex", "claude"} {
		for _, id := range []string{"first", "second"} {
			name := kind + "-" + id
			client := NewClient("tmux")
			var state snapshot.SessionSnapshot
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				state, err = client.CaptureSession(name)
				if err != nil {
					t.Fatal(err)
				}
				if state.Windows[0].Panes[0].Agent != nil {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			pane := state.Windows[0].Panes[0]
			if pane.Agent == nil || pane.Agent.Kind != kind || pane.Agent.ID != id ||
				pane.CurrentPath != "/tmp" {
				t.Fatalf(
					"wrong binding for %s: %+v output=%s",
					name,
					pane,
					testutil.Tmux(t, "capture-pane", "-p", "-t", "="+name+":0.0"),
				)
			}
			verifyHookRoundtrip(t, name, kind, id, client, registry, state)
		}
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("LAZY_TEST_AGENT") != "" {
		_ = flag.Set("test.run", "^TestAgentHookHelper$")
	}
	os.Exit(m.Run())
}

func verifyHookRoundtrip(
	t *testing.T,
	name, kind, id string,
	client *Client,
	registry *integration.Registry,
	state snapshot.SessionSnapshot,
) {
	t.Helper()
	storage := store.New(t.TempDir())
	if err := storage.SaveSession(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := storage.LoadSession(name)
	if err != nil {
		t.Fatal(err)
	}
	command, err := registry.ResolveChecked(loaded.Windows[0].Panes[0])
	if err != nil || !strings.Contains(command, kind+"'") ||
		!strings.Contains(command, "'"+id+"'") {
		t.Fatalf("wrong restore: %s %v", command, err)
	}
	raw := testutil.Tmux(
		t,
		"show-options",
		"-pqv",
		"-t",
		"="+name+":0.0",
		agentBindingOption,
	)
	var binding LiveBinding
	if err := json.Unmarshal([]byte(raw), &binding); err != nil {
		t.Fatal(err)
	}
	binding.StartedAt = "old process"
	body, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(
		t,
		"set-option",
		"-p",
		"-t",
		"="+name+":0.0",
		agentBindingOption,
		string(body),
	)
	stale, err := client.CapturePane("=" + name + ":0.0")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Agent != nil {
		t.Fatal("stale binding accepted")
	}
}
