package antex

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestCaptureDoesNotGuessConversationFromSharedDirectory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	for idx, id := range []string{"01a0d3e1-d263-75c0-8189-c98bc7266f6c", "01a0cee1-2987-7b13-81cc-777ec4861be7"} {
		writeRollout(t, home, "2026/09/24", id, "/workspace", time.Unix(int64(idx), 0))
	}
	registry := integration.NewRegistry(New(home))
	state := snapshot.SessionSnapshot{
		Windows: []snapshot.Window{
			{
				Panes: []snapshot.Pane{
					{
						CurrentCmd:  "antex",
						CurrentPath: "/workspace",
						RestoreCmd:  "antex resume 01a0d3e1-d263-75c0-8189-c98bc7266f6c",
					},
				},
			},
		},
	}
	registry.Enrich(&state)
	pane := state.Windows[0].Panes[0]
	if len(pane.Meta) != 0 {
		t.Fatalf("invented conversation metadata: %v", pane.Meta)
	}
	if got, err := registry.ResolveChecked(pane); err == nil || got != "" {
		t.Fatalf("unverified conversation restored: %q %v", got, err)
	}
}

func TestRestoreUsesPublishedRootAndPreservesArguments(t *testing.T) {
	t.Parallel()
	id := "01a0d3e1-d263-75c0-8189-c98bc7266f6c"
	argv, marshalErr := json.Marshal(
		[]string{"antex", "resume", id, "--profile", "work", "--add-dir", "/path with 'quote'"},
	)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	meta := map[string]string{
		"session_id":        id,
		"session_id_source": "binding-v1",
		"home":              "/home/my user/.antex",
		"resume_argv":       string(argv),
	}
	got, err := New(
		t.TempDir(),
	).RestoreDecision(snapshot.Pane{RestoreCmd: "antex fork another-id"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "'resume' '"+id+"'") ||
		!strings.Contains(got, "'--profile' 'work'") ||
		!strings.Contains(got, `'/path with '"'"'quote'"'"''`) {
		t.Fatalf("incorrect restore command: %s", got)
	}
	if !strings.Contains(got, `tui.resume_cwd="session"`) {
		t.Fatalf("legacy restore may prompt for cwd: %s", got)
	}
	meta["session_id"] = "01a0cee1-2987-7b13-81cc-777ec4861be7"
	if _, err := New(t.TempDir()).RestoreDecision(snapshot.Pane{}, meta); err == nil {
		t.Fatal("conflicting published arguments accepted")
	}
}
