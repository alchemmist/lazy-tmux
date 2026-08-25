package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

var (
	errSessionNameEmpty    = errors.New("session name is empty")
	errWindowNameEmpty     = errors.New("window name is empty")
	errSourceSessionEmpty  = errors.New("source session is empty")
	errWindowNotInSnapshot = errors.New("window not found in snapshot")
	errAlreadyExists       = errors.New("already exists")
	errAlreadyInStorage    = errors.New("already exists in storage")
	errAlreadyAwake        = errors.New("is already awake")
	errNotRunning          = errors.New("is not running")
	errDaemonRunning       = errors.New("daemon already running")
)

type tmuxSessionManager interface {
	ListSessions() ([]string, error)
	SessionsLastAttached() map[string]time.Time
	CurrentSession() (string, error)
	SessionExists(name string) bool
	SocketPath() string
}

type tmuxSessionCapturer interface {
	CaptureSession(name string) (snapshot.SessionSnapshot, error)
	CapturePaneScrollback(target string, lines int) (string, error)
}

type tmuxSessionRestorer interface {
	RestoreSession(ctx context.Context, snap snapshot.SessionSnapshot) error
	SwitchClient(target string) error
	SynchronizeWindowSize(target string) error
	AttachSession(target string) error
	InsideTmux() bool
}

type tmuxSessionMutator interface {
	NewSession(name string) error
	NewWindow(session, name string) error
	KillWindow(session string, windowIndex int) error
	KillSession(session string) error
	RenameWindow(session string, windowIndex int, name string) error
	RenameSession(session, name string) error
}

type tmuxClient interface {
	tmuxSessionManager
	tmuxSessionCapturer
	tmuxSessionRestorer
	tmuxSessionMutator
}

type App struct {
	cfg          config.Config
	store        *store.Store
	tmux         tmuxClient
	integrations *integration.Registry
	saveAllFn    func() error
}

func New(cfg config.Config) *App {
	client := tmux.NewClient(config.ExpandHome(cfg.TmuxBin))
	client.SetRestoreTimeout(cfg.RestoreTimeout)
	client.SetRestoreAllowlist(cfg.RestoreAllowlist)
	client.SetRestoreDenylist(cfg.RestoreDenylist)

	registry := buildRegistry(cfg.Integrations, ClaudeStatusDir(cfg.DataDir))
	client.SetRestoreResolver(registry)

	return &App{
		cfg:          cfg,
		store:        store.New(cfg.DataDir),
		tmux:         client,
		integrations: registry,
	}
}

func ClaudeStatusDir(dataDir string) string {
	return filepath.Join(dataDir, "claude-status")
}

func buildRegistry(cfg config.IntegrationsConfig, statusDir string) *integration.Registry {
	if !cfg.Enabled {
		return integration.NewRegistry()
	}

	var items []integration.Integration

	if cfg.Claude.Enabled {
		items = append(items, claude.New(config.ExpandHome(cfg.Claude.Home), statusDir))
	}

	if cfg.Codex.Enabled {
		items = append(items, codex.New(config.ExpandHome(cfg.Codex.Home)))
	}

	return integration.NewRegistry(items...)
}

func (a *App) SaveAll() (int, error) {
	sessions, err := a.tmux.ListSessions()
	if err != nil {
		return 0, fmt.Errorf("list sessions: %w", err)
	}

	for _, name := range sessions {
		err := a.SaveSession(name)
		if err != nil {
			return 0, err
		}
	}

	return len(sessions), nil
}

func (a *App) SaveSession(session string) error {
	snap, err := a.tmux.CaptureSession(session)
	if err != nil {
		return fmt.Errorf("capture session: %w", err)
	}

	if a.cfg.Scrollback.Enabled {
		a.captureShellScrollback(&snap)
	}

	a.integrations.Enrich(&snap)

	err = a.store.SaveSession(snap)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

func (a *App) SaveCurrent() error {
	name, err := a.tmux.CurrentSession()
	if err != nil {
		return fmt.Errorf("get current session: %w", err)
	}

	return a.SaveSession(name)
}

func (a *App) Restore(session string, switchClient bool) error {
	return a.RestoreTarget(PickerTarget{SessionName: session}, switchClient)
}

func (a *App) RestoreTarget(target PickerTarget, switchClient bool) error {
	preExisted := a.tmux.SessionExists(strings.TrimSpace(target.SessionName))
	err := a.restoreSessionForTarget(context.Background(), target)
	if err != nil {
		return err
	}

	if switchClient {
		return a.handoffToTarget(target, false, !preExisted)
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

			return fmt.Errorf("get latest record: %w", err)
		}

		target = rec.SessionName
	}

	return a.Restore(target, true)
}

func (a *App) ListRecords() ([]snapshot.Record, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	return records, nil
}

func (a *App) restoreSessionForTarget(ctx context.Context, target PickerTarget) error {
	session := strings.TrimSpace(target.SessionName)
	if session == "" {
		return errSessionNameEmpty
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	err = a.tmux.RestoreSession(ctx, snap)
	if err != nil && !errors.Is(err, tmux.ErrSessionExists) {
		return fmt.Errorf("restore session: %w", err)
	}

	err = a.store.MarkSessionAccessed(session, time.Now().UTC())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mark session accessed: %w", err)
	}

	return nil
}

func (a *App) handoffToTarget(
	target PickerTarget,
	allowAttach bool,
	synchronizeSize bool,
) error {
	session := strings.TrimSpace(target.SessionName)

	switchTarget := session
	if target.WindowIndex != nil {
		switchTarget = fmt.Sprintf("%s:%d", session, *target.WindowIndex)
	}

	if a.tmux.InsideTmux() {
		err := a.tmux.SwitchClient(switchTarget)
		if err != nil {
			return fmt.Errorf("switch client: %w", err)
		}
		if synchronizeSize {
			err = a.tmux.SynchronizeWindowSize(switchTarget)
			if err != nil {
				return fmt.Errorf("synchronize window size: %w", err)
			}
		}

		return nil
	}

	if !allowAttach {
		return nil
	}

	err := a.tmux.AttachSession(switchTarget)
	if err != nil {
		return fmt.Errorf("attach session: %w", err)
	}

	return nil
}

func (a *App) runDaemonSaveAll() error {
	if a.saveAllFn != nil {
		return a.saveAllFn()
	}

	_, err := a.SaveAll()

	return err
}

func (a *App) captureShellScrollback(snap *snapshot.SessionSnapshot) {
	lines := a.cfg.Scrollback.Lines
	if lines <= 0 {
		lines = 5000
	}

	for win := range snap.Windows {
		for pi := range snap.Windows[win].Panes {
			pane := &snap.Windows[win].Panes[pi]
			if strings.TrimSpace(pane.RestoreCmd) != "" || !isShellCommandName(pane.CurrentCmd) {
				continue
			}

			target := tmux.PaneTarget(snap.SessionName, snap.Windows[win].Index, pane.Index)

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
				Ref:     "",
				Lines:   0,
				Bytes:   0,
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
