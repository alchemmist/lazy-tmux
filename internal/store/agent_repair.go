package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alchemmist/lazy-tmux/internal/atomicfile"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errSavedPaneMissing = errors.New("saved pane not found")

var errSnapshotVersion = errors.New("unsupported snapshot version")

func (s *Store) RepairAgent(
	session string,
	windowIndex, paneIndex int,
	value snapshot.AgentSession,
	apply bool,
) (snapshot.Pane, error) {
	value.Source = agent.ManualSource
	err := agent.Valid(value)
	if err != nil {
		return snapshot.Pane{}, fmt.Errorf("invalid repaired identity: %w", err)
	}
	unlock, err := s.lockSessionMutation(session)
	if err != nil {
		return snapshot.Pane{}, err
	}
	defer unlock()
	state, err := s.loadSessionUnlocked(session, true)
	if err != nil {
		return snapshot.Pane{}, err
	}
	pane := savedPane(&state, windowIndex, paneIndex)
	if pane == nil {
		return snapshot.Pane{}, errSavedPaneMissing
	}
	pane.Agent = &value
	pane.CurrentCmd = value.Kind
	pane.RestoreCmd = ""
	pane.CurrentPath = value.CWD
	if !apply {
		return *pane, nil
	}
	err = s.backupAgentRepair(session)
	if err != nil {
		return snapshot.Pane{}, err
	}
	err = s.saveSessionUnlocked(state)
	if err != nil {
		return snapshot.Pane{}, err
	}

	return *pane, nil
}

func savedPane(state *snapshot.SessionSnapshot, windowIndex, paneIndex int) *snapshot.Pane {
	for windowIndexInSlice := range state.Windows {
		if state.Windows[windowIndexInSlice].Index != windowIndex {
			continue
		}
		for pi := range state.Windows[windowIndexInSlice].Panes {
			if state.Windows[windowIndexInSlice].Panes[pi].Index == paneIndex {
				return &state.Windows[windowIndexInSlice].Panes[pi]
			}
		}
	}

	return nil
}

func (s *Store) backupAgentRepair(session string) error {
	root, err := os.OpenRoot(filepath.Dir(s.sessionPath(session)))
	if err != nil {
		return fmt.Errorf("open repair directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	name := filepath.Base(s.sessionPath(session))
	original, err := root.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read repair backup: %w", err)
	}
	err = atomicfile.Backup(root, name+".pre-agent-repair.bak", original)
	if err != nil {
		return fmt.Errorf("backup before repair: %w", err)
	}

	return nil
}

func validateSnapshotVersion(data []byte) error {
	var header struct {
		Version int `json:"version"`
	}
	err := json.Unmarshal(data, &header)
	if err != nil {
		return fmt.Errorf("read snapshot version: %w", err)
	}
	if header.Version > snapshot.FormatVersion {
		return fmt.Errorf("%w: %d", errSnapshotVersion, header.Version)
	}

	return nil
}
