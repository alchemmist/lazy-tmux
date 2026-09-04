package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errNoSavedSessions = errors.New("no saved sessions found")

const quickStatusWorkerLimit = 4

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
	statusRegistry := a.integrations.Scope()

	for _, rec := range records {
		snap, err := a.pickerSnapshot(rec)
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
			session.Statuses = windowStatuses(statusRegistry, snap.Windows)
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (a *App) pickerSnapshot(record snapshot.Record) (snapshot.SessionSnapshot, error) {
	a.pickerCacheMu.Lock()
	cached, ok := a.pickerCache[record.SessionName]
	if ok && cached.file == record.File && cached.capturedAt.Equal(record.CapturedAt) {
		a.pickerCacheMu.Unlock()

		return cached.snapshot, nil
	}
	key := pickerCacheKey{
		sessionName: record.SessionName,
		file:        record.File,
		capturedAt:  record.CapturedAt,
	}
	if load, loading := a.pickerLoads[key]; loading {
		a.pickerCacheMu.Unlock()
		<-load.done

		return load.snapshot, load.err
	}
	load := &pickerSnapshotLoad{
		done:     make(chan struct{}),
		snapshot: snapshot.SessionSnapshot{},
		err:      nil,
	}
	a.pickerLoads[key] = load
	a.pickerCacheMisses++
	a.pickerCacheMu.Unlock()

	snap, err := a.store.LoadSessionMetadata(record.SessionName)
	if err != nil {
		err = fmt.Errorf("load picker snapshot: %w", err)
	}
	a.pickerCacheMu.Lock()
	if err == nil {
		a.pickerCache[record.SessionName] = cachedPickerSnapshot{
			capturedAt: record.CapturedAt,
			file:       record.File,
			snapshot:   snap,
		}
	}
	load.snapshot = snap
	load.err = err
	delete(a.pickerLoads, key)
	close(load.done)
	a.pickerCacheMu.Unlock()

	return snap, err
}

func windowStatuses(
	registry *integration.Registry,
	windows []snapshot.Window,
) map[int]picker.WindowStatus {
	statuses := make(map[int]picker.WindowStatus)

	for _, window := range windows {
		for paneIdx := range window.Panes {
			status, ok := registry.Status(window.Panes[paneIdx])
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

func (a *App) SelectQuickSessionWithTUI(initialOffset int) (string, error) {
	sessions, err := a.quickPickerSessions()
	if err != nil {
		return "", err
	}

	session, err := picker.ChooseQuickSession(
		sessions,
		a.cfg.Theme,
		a.cfg.SessionPicker.NavigationModifiers,
		initialOffset,
		func() map[string]bool { return a.quickWorkingSessions(sessions) },
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

	attached := a.tmux.SessionsLastAttached()
	mergeLastAttached(records, attached)
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
			LastAccessed: attached[name],
			Windows:      0,
			Panes:        0,
		})
		byName[name] = struct{}{}
	}
	sortQuickSessionRecords(records, live)

	current, _ := a.tmux.CurrentSession()
	sessions := make([]picker.QuickSession, 0, len(records))
	for _, record := range records {
		_, restored := live[record.SessionName]
		sessions = append(sessions, picker.QuickSession{
			Name:     record.SessionName,
			Restored: restored,
			Current:  record.SessionName == current,
			Working:  false,
		})
	}

	return sessions, nil
}

func (a *App) quickWorkingSessions(sessions []picker.QuickSession) map[string]bool {
	working := make(map[string]bool)
	registry := a.integrations.Scope()
	statuses := parallelMap(
		sessions,
		quickStatusWorkerLimit,
		func(session picker.QuickSession) bool {
			return a.sessionHasWorkingCodex(registry, session.Name, session.Restored)
		},
	)
	for index, isWorking := range statuses {
		if isWorking {
			working[sessions[index].Name] = true
		}
	}

	return working
}

func sortQuickSessionRecords(records []snapshot.Record, live map[string]struct{}) {
	sort.Slice(records, func(indexI, indexJ int) bool {
		_, liveI := live[records[indexI].SessionName]
		_, liveJ := live[records[indexJ].SessionName]
		if liveI != liveJ {
			return liveI
		}
		if !records[indexI].LastAccessed.Equal(records[indexJ].LastAccessed) {
			return records[indexI].LastAccessed.After(records[indexJ].LastAccessed)
		}
		if !records[indexI].CapturedAt.Equal(records[indexJ].CapturedAt) {
			return records[indexI].CapturedAt.After(records[indexJ].CapturedAt)
		}

		return records[indexI].SessionName < records[indexJ].SessionName
	})
}

func (a *App) sessionHasWorkingCodex(
	registry *integration.Registry,
	session string,
	restored bool,
) bool {
	if !restored {
		return false
	}

	snap, err := a.store.LoadSessionMetadata(session)
	if err != nil {
		return false
	}

	for windowIndex := range snap.Windows {
		for paneIndex := range snap.Windows[windowIndex].Panes {
			status, ok := registry.StatusFor(
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
		err := a.handoffToTarget(target, true, false)
		if err != nil {
			return err
		}

		err = a.store.MarkSessionAccessed(session, time.Now().UTC())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("mark session accessed: %w", err)
		}

		return nil
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
