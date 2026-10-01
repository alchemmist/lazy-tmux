package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestSavePreservesLegacyAntexSnapshotBeforeReplacingIt(t *testing.T) {
	t.Parallel()
	store := New(t.TempDir())
	legacy := snapshot.SessionSnapshot{
		SessionName: "work",
		Windows: []snapshot.Window{
			{
				Panes: []snapshot.Pane{
					{
						CurrentCmd: "antex",
						RestoreCmd: "antex resume original",
						Meta: map[string]string{
							"antex.session_id":        "guessed",
							"antex.session_id_source": "cwd",
						},
					},
				},
			},
		},
	}
	if err := store.SaveSession(legacy); err != nil {
		t.Fatal(err)
	}
	path, err := store.SessionPath("work")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := snapshot.SessionSnapshot{
		SessionName: "work",
		Windows:     []snapshot.Window{{Panes: []snapshot.Pane{{CurrentCmd: "zsh"}}}},
	}
	for range 2 {
		if err := store.SaveSession(updated); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := os.ReadFile(path + ".pre-binding-v1.bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("legacy recovery data overwritten")
	}
	records, err := store.ListRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SessionName != "work" {
		t.Fatalf("backup leaked into picker: %v", records)
	}
	if info, err := os.Stat(
		filepath.Clean(path) + ".pre-binding-v1.bak",
	); err != nil ||
		info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions: %v %v", info, err)
	}
}
