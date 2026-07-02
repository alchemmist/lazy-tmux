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

// RestoreTargetAnimated restores target while showing the ASCII loading field in
// the terminal, then hands off (switch/attach). Used by the interactive TUI
// picker so a slow restore shows motion instead of a black popup (#199). Without
// a TTY the animation is a no-op and this behaves like RestoreTarget(_, true).
func (a *App) RestoreTargetAnimated(target PickerTarget) error {
	done := make(chan struct{})

	var restoreErr error

	go func() {
		restoreErr = a.restoreSessionForTarget(target)
		close(done)
	}()

	// Shows the field until done is closed; a no-op (returns immediately) without
	// a TTY, so we always wait on done ourselves before handing off.
	animErr := picker.RunRestoreAnimation(target.SessionName, done)
	<-done

	if restoreErr != nil {
		return restoreErr
	}

	// The animation is cosmetic: a render failure must not skip the hand-off and
	// leave the session restored but never switched/attached. Log and continue.
	if animErr != nil {
		log.Printf("lazy-tmux: restore animation: %v", animErr)
	}

	// allowAttach: the user picked this session interactively and wants to land
	// in it, even from a plain shell outside tmux.
	return a.handoffToTarget(target, true)
}

// RestoreTargetInteractive restores target and hands off, attaching outside tmux
// (the user picked it interactively). Used by the fzf picker engine, which has
// no TUI to draw the loading animation into.
func (a *App) RestoreTargetInteractive(target PickerTarget) error {
	err := a.restoreSessionForTarget(target)
	if err != nil {
		return err
	}

	return a.handoffToTarget(target, true)
}

// SelectTargetWithTUI runs the interactive TUI picker with the default sort
// order and returns the chosen session/window target.
func (a *App) SelectTargetWithTUI() (PickerTarget, error) {
	return a.SelectTargetWithTUISorted(DefaultPickerSortOptions())
}

// SelectTargetWithTUISorted runs the interactive TUI picker sorted by opts and
// returns the chosen target. The picker is wired with the App's CRUD actions
// (delete/rename/new/wakeup/sleep) plus a reload callback. Having no saved
// sessions is not an error: the picker opens empty so the user can create one.
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

// SelectWithTUI runs the TUI picker and returns only the chosen session name,
// discarding any window selection.
func (a *App) SelectWithTUI() (string, error) {
	target, err := a.SelectTargetWithTUI()
	if err != nil {
		return "", err
	}

	return target.SessionName, nil
}

// SelectWithFZF runs the fzf session picker with the default sort order and
// returns the chosen session name.
func (a *App) SelectWithFZF() (string, error) {
	return a.SelectWithFZFSorted(DefaultPickerSortOptions())
}

// SelectWithFZFSorted runs the fzf session picker sorted by opts and returns
// the chosen session name. Unlike the TUI picker, it errors when there are no
// saved sessions.
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
