package picker

import (
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func windowPreviewCommand(w snapshot.Window) string {
	if len(w.Panes) == 0 {
		return ""
	}

	// Snapshot may have sparse pane indices; fall back to first pane if active is missing.
	active := 0

	for i := range w.Panes {
		if w.Panes[i].Index == w.ActivePane {
			active = i
			break
		}
	}

	if cmd := strings.TrimSpace(w.Panes[active].RestoreCmd); cmd != "" {
		return cmd
	}

	return strings.TrimSpace(w.Panes[active].CurrentCmd)
}
