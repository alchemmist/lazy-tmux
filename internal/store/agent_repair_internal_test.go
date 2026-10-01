package store

import (
	"os"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestRepairPreviewAndApplyPreserveSnapshot(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	original := snapshot.SessionSnapshot{
		SessionName: "work",
		Windows: []snapshot.Window{
			{
				Index:  3,
				Name:   "test",
				Layout: "layout",
				Panes: []snapshot.Pane{
					{
						Index:       2,
						CurrentCmd:  "codex",
						CurrentPath: "/workspace",
						Meta:        map[string]string{"other": "keep"},
					},
					{Index: 3, CurrentCmd: "vim"},
				},
			},
		},
	}
	if err := s.SaveSession(original); err != nil {
		t.Fatal(err)
	}
	path, err := s.SessionPath("work")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	value := snapshot.AgentSession{
		Version: 1,
		Kind:    "codex",
		ID:      "chosen",
		Home:    "/home/codex",
		CWD:     "/workspace",
		Argv:    []string{"codex", "resume", "chosen"},
	}
	if _, err := s.RepairAgent("work", 3, 2, value, false); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("preview modified snapshot")
	}
	for range 2 {
		if _, err := s.RepairAgent("work", 3, 2, value, true); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := os.ReadFile(path + ".pre-agent-repair.bak")
	if err != nil || string(backup) != string(before) {
		t.Fatal("backup not preserved")
	}
	got, err := s.LoadSession("work")
	if err != nil {
		t.Fatal(err)
	}
	if got.Windows[0].Layout != "layout" || got.Windows[0].Panes[1].CurrentCmd != "vim" ||
		got.Windows[0].Panes[0].Meta["other"] != "keep" {
		t.Fatal("unrelated fields lost")
	}
	if got.Windows[0].Panes[0].Agent.Source != "manual-v1" {
		t.Fatal("manual repair claimed live verification")
	}
}
