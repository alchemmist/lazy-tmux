package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
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

	mergeLastAttached(records, a.tmux.SessionsLastAttached())

	picker.SortSessionRecords(records, opts.Session)

	return records, nil
}

func mergeLastAttached(records []snapshot.Record, attached map[string]time.Time) {
	if len(attached) == 0 {
		return
	}

	for i := range records {
		when, ok := attached[records[i].SessionName]
		if ok && when.After(records[i].LastAccessed) {
			records[i].LastAccessed = when
		}
	}
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

		if restored {
			session.Statuses = a.windowStatuses(snap.Windows)
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

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

func (a *App) RestoreTargetAnimated(target PickerTarget) (bool, error) {
	preExisted := a.tmux.SessionExists(strings.TrimSpace(target.SessionName))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	var restoreErr error

	go func() {
		restoreErr = a.restoreSessionForTarget(ctx, target)
		close(done)
	}()

	cancelled, animErr := picker.RunRestoreAnimationWithTheme(target.SessionName, done, a.cfg.Theme)
	if cancelled {
		cancel()
		<-done

		if !preExisted {
			a.killPartialRestore(target.SessionName)
		}

		return true, nil
	}

	<-done

	if restoreErr != nil {
		return false, restoreErr
	}

	if animErr != nil {
		log.Printf("lazy-tmux: restore animation: %v", animErr)
	}

	return false, a.handoffToTarget(target, true, !preExisted)
}

func (a *App) killPartialRestore(session string) {
	name := strings.TrimSpace(session)
	if name == "" || !a.tmux.SessionExists(name) {
		return
	}

	err := a.tmux.KillSession(name)
	if err != nil {
		log.Printf("lazy-tmux: cancel restore: kill session %s: %v", name, err)
	}
}

func (a *App) RestoreTargetInteractive(target PickerTarget) error {
	preExisted := a.tmux.SessionExists(strings.TrimSpace(target.SessionName))
	err := a.restoreSessionForTarget(context.Background(), target)
	if err != nil {
		return err
	}

	return a.handoffToTarget(target, true, !preExisted)
}

func (a *App) SelectTargetWithTUI() (PickerTarget, error) {
	return a.SelectTargetWithTUISorted(DefaultPickerSortOptions())
}

func (a *App) SelectTargetWithTUISorted(opts PickerSortOptions) (PickerTarget, error) {
	actions := picker.Actions{
		DeleteWindow:  a.DeleteWindow,
		DeleteSession: a.DeleteSession,
		RenameWindow:  a.RenameWindow,
		RenameSession: a.RenameSession,
		NewSession:    a.NewSession,
		NewWindow:     a.NewWindow,
		Wakeup:        a.Wakeup,
		Sleep:         a.Sleep,
		SetTheme: func(theme string) error {
			return config.SetTheme(config.DefaultConfigPath(), theme)
		},
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

	target, err := picker.ChooseTargetWithLoader(opts.Window, actions, a.cfg.Theme)
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

func (a *App) SelectQuickSessionWithTUI() (string, error) {
	sessions, err := a.quickPickerSessions()
	if err != nil {
		return "", err
	}

	session, err := picker.ChooseQuickSession(
		sessions,
		a.cfg.Theme,
		a.cfg.SessionPicker.NavigationModifiers,
	)
	if err != nil {
		return "", fmt.Errorf("choose quick session: %w", err)
	}

	return session, nil
}

func (a *App) quickPickerSessions() ([]picker.QuickSession, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	liveSessions, err := a.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	byName := make(map[string]struct{}, len(records)+len(liveSessions))
	live := make(map[string]struct{}, len(liveSessions))
	for index := range records {
		byName[records[index].SessionName] = struct{}{}
	}
	for _, name := range liveSessions {
		live[name] = struct{}{}
		if _, ok := byName[name]; ok {
			continue
		}

		records = append(records, snapshot.Record{
			SessionName:  name,
			File:         "",
			CapturedAt:   time.Time{},
			LastAccessed: time.Time{},
			Windows:      0,
			Panes:        0,
		})
		byName[name] = struct{}{}
	}
	sort.Slice(records, func(indexI, indexJ int) bool {
		_, liveI := live[records[indexI].SessionName]
		_, liveJ := live[records[indexJ].SessionName]
		if liveI != liveJ {
			return liveI
		}

		return records[indexI].SessionName < records[indexJ].SessionName
	})

	current, _ := a.tmux.CurrentSession()
	sessions := make([]picker.QuickSession, 0, len(records))
	for _, record := range records {
		_, restored := live[record.SessionName]
		sessions = append(sessions, picker.QuickSession{
			Name:     record.SessionName,
			Restored: restored,
			Current:  record.SessionName == current,
			Working:  a.sessionHasWorkingCodex(record.SessionName, restored),
		})
	}

	return sessions, nil
}

func (a *App) sessionHasWorkingCodex(session string, restored bool) bool {
	if !restored {
		return false
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return false
	}

	for windowIndex := range snap.Windows {
		for paneIndex := range snap.Windows[windowIndex].Panes {
			status, ok := a.integrations.StatusFor(
				"codex",
				snap.Windows[windowIndex].Panes[paneIndex],
			)
			if ok && status == integration.StatusWorking {
				return true
			}
		}
	}

	return false
}

func (a *App) OpenQuickSession(session string) error {
	target := PickerTarget{SessionName: session}
	if a.tmux.SessionExists(strings.TrimSpace(session)) {
		return a.handoffToTarget(target, true, false)
	}

	return a.RestoreTargetInteractive(target)
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
