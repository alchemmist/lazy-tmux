package tmux

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestPiBindingMustNotAcknowledgeDifferentPendingIntent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "saved.jsonl")
	header, err := json.Marshal(map[string]string{"type": "session", "id": "saved", "cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(header, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, id, file string
		pending, valid bool
	}{
		{"matching restore", "saved", file, true, true},
		{"different ID", "other", file, true, false},
		{"different path", "saved", filepath.Join(root, "other.jsonl"), true, false},
		{"no-session child", "saved", "", true, false},
		{"native switch after restore", "old", file, false, true},
		{"legacy identity-only restore", "", file, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pane := snapshot.Pane{
				CurrentPath: root,
				Meta:        map[string]string{"pi.session_file": file, "pi.session_id": tc.id},
			}
			if tc.pending {
				pane.Meta[agent.PendingKey] = "1"
			}
			if got := piRestoreIntentMatches(pane, tc.file); got != tc.valid {
				t.Fatalf("match=%v want %v", got, tc.valid)
			}
		})
	}
	pane := snapshot.Pane{
		CurrentPath: root,
		Meta:        map[string]string{agent.PendingKey: "1"},
		Agent: &snapshot.AgentSession{
			ID:   "other",
			CWD:  root,
			Argv: []string{"pi", "--session", "other", file},
		},
	}
	if piRestoreIntentMatches(pane, file) {
		t.Fatal("binding overwrote pending AgentSession")
	}
	pane.Agent.ID = "saved"
	pane.Agent.Argv[2] = "saved"
	if !piRestoreIntentMatches(pane, file) {
		t.Fatal("matching pending AgentSession rejected")
	}
}

func TestPiNativeSwitchOrForkReplacesStaleSavedIdentity(t *testing.T) {
	t.Parallel()
	for _, liveID := range []string{"new", ""} {
		t.Run("live-id-"+liveID, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			file := filepath.Join(root, "fork.jsonl")
			data, err := json.Marshal(
				map[string]string{"type": "session", "id": "new", "cwd": root},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			pane := snapshot.Pane{
				CurrentCmd:  "pi",
				CurrentPath: root,
				Meta: map[string]string{
					"pi.session_id": "old",
					"pi.cwd":        "/stale",
					"pi.model":      "old-model",
				},
				Agent: &snapshot.AgentSession{Kind: snapshot.AgentPi, ID: "old"},
			}
			bound := applyPiBinding(pane, piBinding{File: file, SessionID: liveID, State: "idle"})
			meta, err := pi.New(root, root).Capture(bound)
			if err != nil || meta["session_id"] != "new" {
				t.Fatalf("new native identity rejected: %#v %v", meta, err)
			}
		})
	}
}
