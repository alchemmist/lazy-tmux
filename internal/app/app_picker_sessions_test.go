package app

import (
	"os"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestPickerSessionsSkipsMissingSnapshotAndMarksRestored(t *testing.T) {
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "list-sessions" ]; then
  echo "alpha"
  exit 0
fi
exit 0
`)

	a := New(config.Config{DataDir: t.TempDir(), TmuxBin: fake})
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)

	for _, name := range []string{"alpha", "beta"} {
		if err := a.store.SaveSession(snapshot.SessionSnapshot{
			Version:     snapshot.FormatVersion,
			SessionName: name,
			CapturedAt:  base,
			Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
		}); err != nil {
			t.Fatalf("save session %q: %v", name, err)
		}
	}

	// Remove beta snapshot file but keep index record to force skip.
	path, err := a.store.SessionPath("beta")
	if err != nil {
		t.Fatalf("SessionPath: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove beta snapshot: %v", err)
	}

	a.tmux = tmux.NewClient(fake)

	sessions, err := a.pickerSessions(DefaultPickerSortOptions())
	if err != nil {
		t.Fatalf("pickerSessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (missing snapshot skipped), got %d", len(sessions))
	}

	if sessions[0].Record.SessionName != "alpha" {
		t.Fatalf("unexpected session name: %s", sessions[0].Record.SessionName)
	}

	if !sessions[0].Restored {
		t.Fatal("expected alpha to be marked as restored")
	}
}
