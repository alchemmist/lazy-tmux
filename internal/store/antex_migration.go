package store

import (
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func migrateAntexSnapshot(state *snapshot.SessionSnapshot) {
	for wi := range state.Windows {
		for pi := range state.Windows[wi].Panes {
			pane := &state.Windows[wi].Panes[pi]
			for key, value := range pane.Meta {
				if suffix, ok := strings.CutPrefix(key, "codex."); ok {
					newKey := "antex." + suffix
					if _, exists := pane.Meta[newKey]; !exists {
						pane.Meta[newKey] = value
					}
					delete(pane.Meta, key)
				}
			}
			for _, command := range []*string{&pane.CurrentCmd, &pane.RestoreCmd} {
				if *command == "codex" {
					*command = "antex"
				} else if suffix, ok := strings.CutPrefix(*command, "codex "); ok {
					*command = "antex " + suffix
				}
			}
		}
	}
}
