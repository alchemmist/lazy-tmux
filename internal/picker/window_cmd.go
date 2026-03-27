package picker

import (
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func windowPreviewCommand(win snapshot.Window) string {
	if len(win.Panes) == 0 {
		return ""
	}

	// Snapshot may have sparse pane indices; fall back to first pane if active is missing.
	active := 0

	for i := range win.Panes {
		if win.Panes[i].Index == win.ActivePane {
			active = i
			break
		}
	}

	if cmd := strings.TrimSpace(win.Panes[active].RestoreCmd); cmd != "" {
		return cmd
	}

	return strings.TrimSpace(win.Panes[active].CurrentCmd)
}
