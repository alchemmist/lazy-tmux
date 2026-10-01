package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func backupLegacyAntex(path string) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open snapshot directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	name := filepath.Base(path)
	data, err := root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy snapshot: %w", err)
	}
	var state snapshot.SessionSnapshot
	if json.Unmarshal(data, &state) != nil || !hasLegacyAntex(state) {
		return nil
	}
	backup := name + ".pre-binding-v1.bak"
	_, err = root.Stat(backup)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy Antex backup: %w", err)
	}
	temporary := backup + ".tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create legacy Antex backup: %w", err)
	}
	defer func() { _ = file.Close(); _ = root.Remove(temporary) }()
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("write legacy Antex backup: %w", err)
	}
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync legacy Antex backup: %w", err)
	}
	err = root.Link(temporary, backup)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("publish legacy Antex backup: %w", err)
	}

	return nil
}

func hasLegacyAntex(state snapshot.SessionSnapshot) bool {
	for _, window := range state.Windows {
		for _, pane := range window.Panes {
			if pane.Agent != nil {
				continue
			}
			if pane.Meta["antex.session_id"] != "" || pane.Meta["codex.session_id"] != "" ||
				pane.Meta["claude.session_id"] != "" {
				return true
			}
			for _, command := range []string{pane.CurrentCmd, pane.RestoreCmd} {
				fields := strings.Fields(command)
				if len(fields) > 0 &&
					agent.Kind(fields[0]) != "" {
					return true
				}
			}
		}
	}

	return false
}
