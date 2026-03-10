package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if a.cfg.Scrollback.Enabled {
		a.captureShellScrollback(&snap)
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

func (a *App) captureShellScrollback(snap *snapshot.SessionSnapshot) {
	lines := a.cfg.Scrollback.Lines
	if lines <= 0 {
		lines = 5000
	}

	for wi := range snap.Windows {
		for pi := range snap.Windows[wi].Panes {
			pane := &snap.Windows[wi].Panes[pi]
			if strings.TrimSpace(pane.RestoreCmd) != "" || !isShellCommandName(pane.CurrentCmd) {
				continue
			}
			target := tmux.PaneTarget(snap.SessionName, snap.Windows[wi].Index, pane.Index)
			out, err := a.tmux.CapturePaneScrollback(target, lines)
			if err != nil {
				continue
			}
			out = strings.TrimRight(out, "\n")
			if strings.TrimSpace(out) == "" {
				continue
			}
			pane.Scrollback = &snapshot.ScrollbackRef{
				Content: out + "\n",
			}
		}
	}
}

func isShellCommandName(cmd string) bool {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return false
	}
	base := strings.TrimPrefix(filepath.Base(fields[0]), "-")
	switch base {
	case "bash", "zsh", "fish", "sh", "ksh":
		return true
	default:
		return false
	}
}
