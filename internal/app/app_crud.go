package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func (a *App) DeleteWindow(session string, windowIndex int) error {
	if a.tmux.SessionExists(session) {
		if err := a.tmux.KillWindow(session, windowIndex); err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return err
			}
		} else {
			if !a.tmux.SessionExists(session) {
				return a.store.DeleteSession(session)
			}
			return a.SaveSession(session)
		}
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("session %q not found: %w", session, os.ErrNotExist)
		}
		return err
	}
	windows := make([]snapshot.Window, 0, len(snap.Windows))
	removed := false
	for _, w := range snap.Windows {
		if w.Index == windowIndex {
			removed = true
			continue
		}
		windows = append(windows, w)
	}
	if !removed {
		return fmt.Errorf("window not found in snapshot")
	}
	if len(windows) == 0 {
		return a.store.DeleteSession(session)
	}
	snap.Windows = windows
	return a.store.SaveSession(snap)
}

func (a *App) DeleteSession(session string) error {
	if a.tmux.SessionExists(session) {
		if err := a.tmux.KillSession(session); err != nil {
			return err
		}
	}
	return a.store.DeleteSession(session)
}

func (a *App) RenameWindow(session string, windowIndex int, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("window name is empty")
	}
	if a.tmux.SessionExists(session) {
		if err := a.tmux.RenameWindow(session, windowIndex, name); err != nil {
			return err
		}
	}
	snap, err := a.store.LoadSession(session)
	if err != nil {
		return err
	}
	updated := false
	for i := range snap.Windows {
		if snap.Windows[i].Index == windowIndex {
			snap.Windows[i].Name = name
			updated = true
			break
		}
	}
	if !updated {
		return fmt.Errorf("window not found in snapshot")
	}
	return a.store.SaveSession(snap)
}

func (a *App) RenameSession(session string, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("session name is empty")
	}
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("source session is empty")
	}
	if session == name {
		return nil
	}
	srcPath, err := a.store.SessionPath(session)
	if err != nil {
		return err
	}
	dstPath, err := a.store.SessionPath(name)
	if err != nil {
		return err
	}
	if srcPath == dstPath {
		return nil
	}
	exists, err := a.store.SessionExists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("session %q already exists", name)
	}
	snap, err := a.store.LoadSession(session)
	if err != nil {
		return err
	}
	snap.SessionName = name
	if a.tmux.SessionExists(session) {
		if err := a.tmux.RenameSession(session, name); err != nil {
			return err
		}
	}
	if err := a.store.SaveSession(snap); err != nil {
		return err
	}
	return a.store.DeleteSession(session)
}

func (a *App) NewSession(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("session name is empty")
	}
	if exists, err := a.store.SessionExists(name); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("session %q already exists in storage", name)
	}
	if err := a.tmux.NewSession(name); err != nil {
		return err
	}
	snap, err := a.tmux.CaptureSession(name)
	if err != nil {
		_ = a.tmux.KillSession(name)
		return err
	}
	if err := a.store.SaveSession(snap); err != nil {
		_ = a.tmux.KillSession(name)
		return err
	}
	return nil
}

func (a *App) NewWindow(session string, name string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is empty")
	}
	if a.tmux.SessionExists(session) {
		if err := a.tmux.NewWindow(session, name); err != nil {
			return err
		}
		snap, err := a.tmux.CaptureSession(session)
		if err != nil {
			return err
		}
		return a.store.SaveSession(snap)
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return err
	}
	idx := nextWindowIndex(snap.Windows)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("window-%d", idx)
	}
	snap.Windows = append(snap.Windows, snapshot.Window{
		Index:      idx,
		Name:       name,
		ActivePane: 0,
		Panes: []snapshot.Pane{
			{Index: 0},
		},
	})
	return a.store.SaveSession(snap)
}

func nextWindowIndex(windows []snapshot.Window) int {
	maxIdx := -1
	for _, w := range windows {
		if w.Index > maxIdx {
			maxIdx = w.Index
		}
	}
	return maxIdx + 1
}

func (a *App) Wakeup(session string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is empty")
	}
	// Check if session already exists
	if a.tmux.SessionExists(session) {
		return fmt.Errorf("session %q is already awake", session)
	}
	// Restore the session
	return a.Restore(session, false)
}

func (a *App) Sleep(session string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is empty")
	}
	// Check if session exists
	if !a.tmux.SessionExists(session) {
		return fmt.Errorf("session %q is not running", session)
	}
	// Save the session first
	if err := a.SaveSession(session); err != nil {
		return err
	}
	// Then kill it
	return a.tmux.KillSession(session)
}
