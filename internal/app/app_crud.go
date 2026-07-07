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
		handled, err := a.deleteLiveWindow(session, windowIndex)
		if handled {
			return err
		}
	}

	return a.deleteSnapshotWindow(session, windowIndex)
}

func (a *App) deleteLiveWindow(session string, windowIndex int) (bool, error) {
	err := a.tmux.KillWindow(session, windowIndex)
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return true, fmt.Errorf("kill window: %w", err)
		}

		return false, nil
	}

	if !a.tmux.SessionExists(session) {
		err := a.store.DeleteSession(session)
		if err != nil {
			return true, fmt.Errorf("delete session: %w", err)
		}

		return true, nil
	}

	return true, a.SaveSession(session)
}

func (a *App) deleteSnapshotWindow(session string, windowIndex int) error {
	snap, err := a.store.LoadSession(session)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("session %q not found: %w", session, os.ErrNotExist)
		}

		return fmt.Errorf("load session: %w", err)
	}

	windows := make([]snapshot.Window, 0, len(snap.Windows))
	removed := false

	for _, write := range snap.Windows {
		if write.Index == windowIndex {
			removed = true

			continue
		}

		windows = append(windows, write)
	}

	if !removed {
		return errWindowNotInSnapshot
	}

	if len(windows) == 0 {
		err := a.store.DeleteSession(session)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
		}

		return nil
	}

	snap.Windows = windows

	err = a.store.SaveSession(snap)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

func (a *App) Forget(session string) error {
	if strings.TrimSpace(session) == "" {
		return errSessionNameEmpty
	}

	err := a.store.DeleteSession(strings.TrimSpace(session))
	if err != nil {
		return fmt.Errorf("delete session storage: %w", err)
	}

	return nil
}

func (a *App) DeleteSession(session string) error {
	if a.tmux.SessionExists(session) {
		err := a.tmux.KillSession(session)
		if err != nil {
			return fmt.Errorf("kill session: %w", err)
		}
	}

	err := a.store.DeleteSession(session)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (a *App) RenameWindow(session string, windowIndex int, name string) error {
	if strings.TrimSpace(name) == "" {
		return errWindowNameEmpty
	}

	if a.tmux.SessionExists(session) {
		err := a.tmux.RenameWindow(session, windowIndex, name)
		if err != nil {
			return fmt.Errorf("rename window: %w", err)
		}
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
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
		return errWindowNotInSnapshot
	}

	err = a.store.SaveSession(snap)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

func (a *App) RenameSession(session, name string) error {
	if strings.TrimSpace(name) == "" {
		return errSessionNameEmpty
	}

	if strings.TrimSpace(session) == "" {
		return errSourceSessionEmpty
	}

	if session == name {
		return nil
	}

	srcPath, err := a.store.SessionPath(session)
	if err != nil {
		return fmt.Errorf("get source session path: %w", err)
	}

	dstPath, err := a.store.SessionPath(name)
	if err != nil {
		return fmt.Errorf("get dest session path: %w", err)
	}

	if srcPath == dstPath {
		return nil
	}

	exists, err := a.store.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session exists: %w", err)
	}

	if exists {
		return fmt.Errorf("session %q %w", name, errAlreadyExists)
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	snap.SessionName = name

	if a.tmux.SessionExists(session) {
		err := a.tmux.RenameSession(session, name)
		if err != nil {
			return fmt.Errorf("rename tmux session: %w", err)
		}
	}

	err = a.store.SaveSession(snap)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	err = a.store.DeleteSession(session)
	if err != nil {
		return fmt.Errorf("delete old session: %w", err)
	}

	return nil
}

func (a *App) NewSession(name string) error {
	if strings.TrimSpace(name) == "" {
		return errSessionNameEmpty
	}

	exists, err := a.store.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session exists: %w", err)
	} else if exists {
		return fmt.Errorf("session %q %w", name, errAlreadyInStorage)
	}

	err = a.tmux.NewSession(name)
	if err != nil {
		return fmt.Errorf("create new session: %w", err)
	}

	snap, err := a.tmux.CaptureSession(name)
	if err != nil {
		_ = a.tmux.KillSession(name)

		return fmt.Errorf("capture session: %w", err)
	}

	err = a.store.SaveSession(snap)
	if err != nil {
		_ = a.tmux.KillSession(name)

		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

func (a *App) NewWindow(session, name string) error {
	if strings.TrimSpace(session) == "" {
		return errSessionNameEmpty
	}

	if a.tmux.SessionExists(session) {
		err := a.tmux.NewWindow(session, name)
		if err != nil {
			return fmt.Errorf("create new window: %w", err)
		}

		snap, err := a.tmux.CaptureSession(session)
		if err != nil {
			return fmt.Errorf("capture session: %w", err)
		}

		err = a.store.SaveSession(snap)
		if err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		return nil
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	idx := nextWindowIndex(snap.Windows)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("window-%d", idx)
	}

	snap.Windows = append(snap.Windows, snapshot.Window{
		Index:      idx,
		Name:       name,
		Layout:     "",
		IsActive:   false,
		ActivePane: 0,
		Panes: []snapshot.Pane{
			{
				Index:       0,
				IsActive:    false,
				CurrentPath: "",
				CurrentCmd:  "",
				RestoreCmd:  "",
				Scrollback:  nil,
				Meta:        nil,
			},
		},
	})

	err = a.store.SaveSession(snap)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
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
		return errSessionNameEmpty
	}
	if a.tmux.SessionExists(session) {
		return fmt.Errorf("session %q %w", session, errAlreadyAwake)
	}

	return a.Restore(session, false)
}

func (a *App) Sleep(session string) error {
	if strings.TrimSpace(session) == "" {
		return errSessionNameEmpty
	}
	if !a.tmux.SessionExists(session) {
		return fmt.Errorf("session %q %w", session, errNotRunning)
	}
	err := a.SaveSession(session)
	if err != nil {
		return err
	}
	err = a.tmux.KillSession(session)
	if err != nil {
		return fmt.Errorf("kill session: %w", err)
	}

	return nil
}
