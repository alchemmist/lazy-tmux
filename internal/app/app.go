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

func (a *App) DeleteWindow(session string, windowIndex int) error {
	if a.tmux.SessionExists(session) {
		if err := a.tmux.KillWindow(session, windowIndex); err != nil {
			return err
		}
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
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
	return a.tmux.KillSession(session)
}

func (a *App) RenameWindow(session string, windowIndex int, name string) error {
	return a.tmux.RenameWindow(session, windowIndex, name)
}

func (a *App) RenameSession(session string, name string) error {
	return a.tmux.RenameSession(session, name)
}

func (a *App) NewSession(name string) error {
	return a.tmux.NewSession(name)
}

func (a *App) NewWindow(session string, name string) error {
	return a.tmux.NewWindow(session, name)
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

func (a *App) pickerRecords(opts PickerSortOptions) ([]snapshot.Record, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no saved sessions found")
	}
	sortSessionRecords(records, opts.Session)
	return records, nil
}

func (a *App) pickerSessions(opts PickerSortOptions) ([]pickerSession, error) {
	records, err := a.pickerRecords(opts)
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
	return a.SelectTargetWithTUISorted(DefaultPickerSortOptions())
}

func (a *App) SelectTargetWithTUISorted(opts PickerSortOptions) (PickerTarget, error) {
	sessions, err := a.pickerSessions(opts)
	if err != nil {
		return PickerTarget{}, err
	}
	actions := pickerActions{
		DeleteWindow:  a.DeleteWindow,
		DeleteSession: a.DeleteSession,
		RenameWindow:  a.RenameWindow,
		RenameSession: a.RenameSession,
		NewSession:    a.NewSession,
		NewWindow:     a.NewWindow,
		Reload: func() ([]pickerSession, error) {
			return a.pickerSessions(opts)
		},
	}
	return chooseTarget(sessions, opts.Window, actions)
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
	return chooseSessionFZF(records)
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
			target := fmt.Sprintf("%s:%d.%d", snap.SessionName, snap.Windows[wi].Index, pane.Index)
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
