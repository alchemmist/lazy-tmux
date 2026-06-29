package app

import (
	"errors"
	"fmt"
	"log"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errNoSavedSessions = errors.New("no saved sessions found")

func (a *App) pickerRecords(opts PickerSortOptions) ([]snapshot.Record, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	if len(records) == 0 {
		return nil, errNoSavedSessions
	}

	picker.SortSessionRecords(records, opts.Session)

	return records, nil
}

func (a *App) pickerSessions(opts PickerSortOptions) ([]picker.Session, error) {
	records, err := a.pickerRecords(opts)
	if err != nil {
		return nil, err
	}

	liveSessions, err := a.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	live := make(map[string]struct{}, len(liveSessions))
	for _, name := range liveSessions {
		live[name] = struct{}{}
	}

	sessions := make([]picker.Session, 0, len(records))

	for _, rec := range records {
		snap, err := a.store.LoadSession(rec.SessionName)
		if err != nil {
			log.Printf("picker: skip session %s: %v", rec.SessionName, err)
			continue
		}

		_, restored := live[rec.SessionName]

		session := picker.Session{
			Record:   rec,
			Windows:  snap.Windows,
			Restored: restored,
		}

		// Live program status (e.g. Claude working / awaiting input) only makes
		// sense for sessions currently running in tmux.
		if restored {
			session.Statuses = a.windowStatuses(snap.Windows)
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

// windowStatuses resolves the live integration status of each window (keyed by
// window index) from its panes. Reads are cheap and best-effort; windows with no
// reported status are omitted.
func (a *App) windowStatuses(windows []snapshot.Window) map[int]picker.WindowStatus {
	statuses := make(map[int]picker.WindowStatus)

	for _, window := range windows {
		for paneIdx := range window.Panes {
			status, ok := a.integrations.Status(window.Panes[paneIdx])
			if !ok {
				continue
			}

			statuses[window.Index] = toPickerStatus(status)

			break
		}
	}

	return statuses
}

func toPickerStatus(status integration.Status) picker.WindowStatus {
	switch status {
	case integration.StatusWorking:
		return picker.StatusWorking
	case integration.StatusAwaitingDecision:
		return picker.StatusAwaitingDecision
	case integration.StatusAwaitingInput:
		return picker.StatusAwaitingInput
	case integration.StatusIdle:
		return picker.StatusIdle
	case integration.StatusError:
		return picker.StatusError
	default:
		return picker.StatusNone
	}
}

func (a *App) SelectTargetWithTUI() (PickerTarget, error) {
	return a.SelectTargetWithTUISorted(DefaultPickerSortOptions())
}

func (a *App) SelectTargetWithTUISorted(opts PickerSortOptions) (PickerTarget, error) {
	sessions, err := a.pickerSessions(opts)
	if err != nil {
		if errors.Is(err, errNoSavedSessions) {
			sessions = []picker.Session{}
		} else {
			return PickerTarget{}, err
		}
	}

	actions := picker.Actions{
		DeleteWindow:  a.DeleteWindow,
		DeleteSession: a.DeleteSession,
		RenameWindow:  a.RenameWindow,
		RenameSession: a.RenameSession,
		NewSession:    a.NewSession,
		NewWindow:     a.NewWindow,
		Wakeup:        a.Wakeup,
		Sleep:         a.Sleep,
		Reload: func() ([]picker.Session, error) {
			sessions, err := a.pickerSessions(opts)
			if err != nil {
				if errors.Is(err, errNoSavedSessions) {
					return []picker.Session{}, nil
				}

				return nil, err
			}

			return sessions, nil
		},
	}

	target, err := picker.ChooseTarget(sessions, opts.Window, actions)
	if err != nil {
		return picker.Target{}, fmt.Errorf("choose target: %w", err)
	}

	return target, nil
}

func (a *App) SelectWithTUI() (string, error) {
	target, err := a.SelectTargetWithTUI()
	if err != nil {
		return "", err
	}

	return target.SessionName, nil
}

func (a *App) SelectWithFZF() (string, error) {
	return a.SelectWithFZFSorted(DefaultPickerSortOptions())
}

func (a *App) SelectWithFZFSorted(opts PickerSortOptions) (string, error) {
	records, err := a.pickerRecords(opts)
	if err != nil {
		return "", err
	}

	session, err := picker.ChooseSessionFZF(records)
	if err != nil {
		return "", fmt.Errorf("choose session fzf: %w", err)
	}

	return session, nil
}

// SelectTargetWithFZFSorted presents a flat, window-level fzf list and returns
// the chosen window as a target (session + window). It is the fzf-engine
// counterpart that lets users jump straight to a specific window.
func (a *App) SelectTargetWithFZFSorted(opts PickerSortOptions) (PickerTarget, error) {
	sessions, err := a.pickerSessions(opts)
	if err != nil {
		return PickerTarget{}, err
	}

	target, err := picker.ChooseWindowFZF(sessions, opts.Window)
	if err != nil {
		return PickerTarget{}, fmt.Errorf("choose window fzf: %w", err)
	}

	return target, nil
}
