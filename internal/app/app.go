package app

import (
	"fmt"
	"os"
	"sort"
	"strings"

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
	snap, err := a.store.LoadSession(session)
	if err != nil {
		return err
	}
	err = a.tmux.RestoreSession(snap)
	if err != nil && err != tmux.ErrSessionExists {
		return err
	}
	if switchClient {
		return a.tmux.SwitchClient(session)
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

func (a *App) SelectWithFZF() (string, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("no saved sessions found")
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CapturedAt.After(records[j].CapturedAt) })
	return chooseSession(records)
}
