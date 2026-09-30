package store

import (
	"reflect"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestLegacyAntexSnapshotPreservesLayoutAndUnrelatedCommands(t *testing.T) {
	t.Parallel()

	state := snapshot.SessionSnapshot{
		Version: 1, SessionName: "arcadia", CurrentWin: 4, CurrentPane: 2,
		Windows: []snapshot.Window{{
			Index: 4, Name: "cpu", Layout: "layout", ActivePane: 2,
			Panes: []snapshot.Pane{
				{
					Index:       2,
					CurrentPath: "/work",
					CurrentCmd:  "codex",
					RestoreCmd:  "codex resume thread-id",
					Meta: map[string]string{
						"codex.session_id": "thread-id",
						"other.key":        "keep",
					},
				},
				{Index: 3, CurrentCmd: "bash", RestoreCmd: "printf codex"},
			},
		}},
	}
	store := New(t.TempDir())
	if err := store.SaveSession(state); err != nil {
		t.Fatal(err)
	}
	expected := state
	var err error
	state, err = store.LoadSession(state.SessionName)
	if err != nil {
		t.Fatal(err)
	}
	expected.Version = state.Version
	expected.CapturedAt = state.CapturedAt
	if !reflect.DeepEqual(state, expected) {
		t.Fatalf("unexpected migrated snapshot: %#v", state)
	}
}
