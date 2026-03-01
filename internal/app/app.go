package app

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

type App struct {
	cfg   config.Config
	store *store.Store
	tmux  *tmux.Client
}

type PickerTarget struct {
	SessionName string
	WindowIndex *int
}

type pickerSession struct {
	Record   snapshot.Record
	Windows  []snapshot.Window
	Restored bool
}

func New(cfg config.Config) *App {
	return &App{
		cfg:   cfg,
		store: store.New(cfg.DataDir),
		tmux:  tmux.NewClient(cfg.TmuxBin),
	}
}

func (a *App) SaveAll() error {
	sessions, err := a.tmux.ListSessions()
	if err != nil {
		return err
	}
	for _, name := range sessions {
		if err := a.SaveSession(name); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) SaveSession(session string) error {
	snap, err := a.tmux.CaptureSession(session)
	if err != nil {
		return err
	}
	return a.store.SaveSession(snap)
}

func (a *App) SaveCurrent() error {
	name, err := a.tmux.CurrentSession()
	if err != nil {
		return err
	}
	return a.SaveSession(name)
}

func (a *App) Restore(session string, switchClient bool) error {
	return a.RestoreTarget(PickerTarget{SessionName: session}, switchClient)
}

func (a *App) RestoreTarget(target PickerTarget, switchClient bool) error {
	session := strings.TrimSpace(target.SessionName)
	if session == "" {
		return fmt.Errorf("empty session name")
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return err
	}
	err = a.tmux.RestoreSession(snap)
	if err != nil && err != tmux.ErrSessionExists {
		return err
	}
	if switchClient {
		switchTarget := session
		if target.WindowIndex != nil {
			switchTarget = fmt.Sprintf("%s:%d", session, *target.WindowIndex)
		}
		if err := a.tmux.SwitchClient(switchTarget); err != nil {
			return err
		}
	}
	if err := a.store.MarkSessionAccessed(session, time.Now().UTC()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (a *App) Bootstrap(session string) error {
	target := strings.TrimSpace(session)
	if target == "" || target == "last" {
		rec, err := a.store.LatestRecord()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		target = rec.SessionName
	}
	return a.Restore(target, true)
}

func (a *App) ListRecords() ([]snapshot.Record, error) {
	return a.store.ListRecords()
}

func (a *App) pickerRecords() ([]snapshot.Record, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no saved sessions found")
	}
	sort.Slice(records, func(i, j int) bool {
		if !records[i].LastAccessed.Equal(records[j].LastAccessed) {
			return records[i].LastAccessed.After(records[j].LastAccessed)
		}
		if !records[i].CapturedAt.Equal(records[j].CapturedAt) {
			return records[i].CapturedAt.After(records[j].CapturedAt)
		}
		return records[i].SessionName < records[j].SessionName
	})
	return records, nil
}

func (a *App) pickerSessions() ([]pickerSession, error) {
	records, err := a.pickerRecords()
	if err != nil {
		return nil, err
	}
	liveSessions, err := a.tmux.ListSessions()
	if err != nil {
		return nil, err
	}
	live := make(map[string]struct{}, len(liveSessions))
	for _, name := range liveSessions {
		live[name] = struct{}{}
	}

	sessions := make([]pickerSession, 0, len(records))
	for _, rec := range records {
		snap, err := a.store.LoadSession(rec.SessionName)
		if err != nil {
			return nil, err
		}
		_, restored := live[rec.SessionName]
		sessions = append(sessions, pickerSession{
			Record:   rec,
			Windows:  snap.Windows,
			Restored: restored,
		})
	}
	return sessions, nil
}

func (a *App) SelectTargetWithTUI() (PickerTarget, error) {
	sessions, err := a.pickerSessions()
	if err != nil {
		return PickerTarget{}, err
	}
	return chooseTarget(sessions)
}

func (a *App) SelectWithTUI() (string, error) {
	target, err := a.SelectTargetWithTUI()
	if err != nil {
		return "", err
	}
	return target.SessionName, nil
}

func (a *App) SelectWithFZF() (string, error) {
	records, err := a.pickerRecords()
	if err != nil {
		return "", err
	}
	return chooseSessionFZF(records)
}
