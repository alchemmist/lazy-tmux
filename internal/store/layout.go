package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// SaveLayoutCheckpoint deliberately uses a separate schema and directory: a
// partial capture must never be discovered by restore, sleep, or the index.
func (s *Store) SaveLayoutCheckpoint(snap snapshot.SessionSnapshot, reason string) (string, error) {
	type pane struct {
		Index int    `json:"index"`
		Path  string `json:"cwd"`
	}
	type window struct {
		Index  int    `json:"index"`
		Name   string `json:"name"`
		Layout string `json:"layout"`
		Panes  []pane `json:"panes"`
	}
	checkpoint := struct {
		Session    string    `json:"session"`
		CapturedAt time.Time `json:"captured_at"`
		Reason     string    `json:"incomplete_reason"`
		Windows    []window  `json:"windows"`
	}{Session: snap.SessionName, CapturedAt: snap.CapturedAt, Reason: reason, Windows: nil}
	for _, w := range snap.Windows {
		win := window{Index: w.Index, Name: w.Name, Layout: w.Layout, Panes: nil}
		for _, p := range w.Panes {
			win.Panes = append(win.Panes, pane{Index: p.Index, Path: p.CurrentPath})
		}
		checkpoint.Windows = append(checkpoint.Windows, win)
	}
	unlock, err := s.lockSessionMutation(snap.SessionName)
	if err != nil {
		return "", err
	}
	defer unlock()
	dir := filepath.Join(s.baseDir, "recovery", "layouts")
	err = os.MkdirAll(dir, 0o700)
	if err != nil {
		return "", fmt.Errorf("create checkpoint directory: %w", err)
	}
	path := filepath.Join(dir, sanitizeName(snap.SessionName)+".json")
	tmp, err := writeJSONTemp(path, checkpoint, 0o600)
	if err != nil {
		return "", err
	}
	err = os.Rename(tmp, path)
	if err != nil {
		return "", fmt.Errorf("publish checkpoint: %w", err)
	}

	return path, nil
}
