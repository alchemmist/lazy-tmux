package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

type tmuxSessionManager interface {
	ListSessions() ([]string, error)
	CurrentSession() (string, error)
	SessionExists(name string) bool
	SocketPath() string
}

type tmuxSessionCapturer interface {
	CaptureSession(name string) (snapshot.SessionSnapshot, error)
	CapturePaneScrollback(target string, lines int) (string, error)
}

type tmuxSessionRestorer interface {
	RestoreSession(snap snapshot.SessionSnapshot) error
	SwitchClient(target string) error
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
	// Expand a leading ~ so both tmux_bin (TOML) and --tmux-bin (flag) accept
	// "~/bin/tmux.appimage"; exec does not do shell tilde expansion.
	client := tmux.NewClient(config.ExpandHome(cfg.TmuxBin))
	client.SetRestoreTimeout(cfg.RestoreTimeout)
	client.SetRestoreAllowlist(cfg.RestoreAllowlist)

	registry := buildRegistry(cfg.Integrations)
	client.SetRestoreResolver(registry)

	return &App{
		cfg:          cfg,
		store:        store.New(cfg.DataDir),
		tmux:         client,
		integrations: registry,
	}
}

// buildRegistry assembles the enabled program integrations from config. With the
// master switch off it returns an empty (inert) registry.
func buildRegistry(cfg config.IntegrationsConfig) *integration.Registry {
	if !cfg.Enabled {
		return integration.NewRegistry()
	}

	var items []integration.Integration

	if cfg.Claude.Enabled {
		items = append(items, claude.New(config.ExpandHome(cfg.Claude.Home)))
	}

	return integration.NewRegistry(items...)
}

// SaveAll snapshots every running tmux session and returns how many were saved
// (0 when no tmux server is running or it has no sessions).
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

	// Enrich panes with program-integration metadata (e.g. the Claude session
	// id) so restore can replay a smarter command. Inert when disabled.
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
	session := strings.TrimSpace(target.SessionName)
	if session == "" {
		return fmt.Errorf("empty session name")
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	err = a.tmux.RestoreSession(snap)
	if err != nil && err != tmux.ErrSessionExists {
		return fmt.Errorf("restore session: %w", err)
	}

	if switchClient {
		switchTarget := session
		if target.WindowIndex != nil {
			switchTarget = fmt.Sprintf("%s:%d", session, *target.WindowIndex)
		}

		err := a.tmux.SwitchClient(switchTarget)
		if err != nil {
			return fmt.Errorf("switch client: %w", err)
		}
	}

	err = a.store.MarkSessionAccessed(
		session,
		time.Now().UTC(),
	)
	if err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mark session accessed: %w", err)
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
