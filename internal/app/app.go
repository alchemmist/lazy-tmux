// Package app is the orchestration layer of lazy-tmux: it wires the tmux
// client, snapshot store, config and picker together into the user-level
// operations behind the CLI commands (save, restore, bootstrap, daemon,
// session/window CRUD and the interactive pickers).
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

// Sentinel errors of session CRUD; dynamic details wrap them so callers can
// match with errors.Is while the rendered messages stay unchanged.
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

// App bundles the tmux client, snapshot store, config and program-integration
// registry, and exposes lazy-tmux's user-level operations on top of them.
type App struct {
	cfg          config.Config
	store        *store.Store
	tmux         tmuxClient
	integrations *integration.Registry
	saveAllFn    func() error
}

// New builds an App from cfg: a tmux client (with ~ in TmuxBin expanded and the
// restore timeout/allowlist/denylist applied), a snapshot store rooted at
// cfg.DataDir, and the enabled program integrations wired in as the restore
// command resolver.
func New(cfg config.Config) *App {
	// Expand a leading ~ so both tmux_bin (TOML) and --tmux-bin (flag) accept
	// "~/bin/tmux.appimage"; exec does not do shell tilde expansion.
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

// ClaudeStatusDir is where the `lazy-tmux hook claude-status` command writes
// per-project Claude status files, derived from the data dir.
func ClaudeStatusDir(dataDir string) string {
	return filepath.Join(dataDir, "claude-status")
}

// buildRegistry assembles the enabled program integrations from config. With the
// master switch off it returns an empty (inert) registry. statusDir is where the
// Claude integration reads hook-written live-status files.
func buildRegistry(cfg config.IntegrationsConfig, statusDir string) *integration.Registry {
	if !cfg.Enabled {
		return integration.NewRegistry()
	}

	var items []integration.Integration

	if cfg.Claude.Enabled {
		items = append(items, claude.New(config.ExpandHome(cfg.Claude.Home), statusDir))
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

// SaveSession captures the named tmux session and persists its snapshot,
// including shell-pane scrollback (when enabled) and program-integration
// metadata such as the Claude session id.
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

// SaveCurrent saves the tmux session the caller is currently attached to; it
// fails when invoked outside tmux.
func (a *App) SaveCurrent() error {
	name, err := a.tmux.CurrentSession()
	if err != nil {
		return fmt.Errorf("get current session: %w", err)
	}

	return a.SaveSession(name)
}

// Restore restores the named session from its snapshot; see RestoreTarget for
// the switchClient semantics.
func (a *App) Restore(session string, switchClient bool) error {
	return a.RestoreTarget(PickerTarget{SessionName: session}, switchClient)
}

// RestoreTarget restores the target's session from its snapshot (a session that
// already exists is not an error) and marks it accessed. With switchClient set
// it also switches the current tmux client to the target; outside tmux it never
// attaches, keeping CLI restore/bootstrap scriptable.
func (a *App) RestoreTarget(target PickerTarget, switchClient bool) error {
	err := a.restoreSessionForTarget(target)
	if err != nil {
		return err
	}

	if switchClient {
		// CLI restore/bootstrap: switch inside tmux, but never attach outside it —
		// keep it scriptable.
		return a.handoffToTarget(target, false)
	}

	return nil
}

// Bootstrap restores one session at tmux startup: the named one, or with ""
// or "last" the most recently used snapshot. Having no snapshots at all is not
// an error — it is a normal first run and Bootstrap silently does nothing.
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

// ListRecords returns the metadata records of all stored session snapshots.
func (a *App) ListRecords() ([]snapshot.Record, error) {
	records, err := a.store.ListRecords()
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	return records, nil
}

// restoreSessionForTarget loads and restores the session (creating it if needed)
// and marks it accessed. It does not switch or attach — that is handoffToTarget,
// kept separate so the loading animation can run between the two.
func (a *App) restoreSessionForTarget(target PickerTarget) error {
	session := strings.TrimSpace(target.SessionName)
	if session == "" {
		return errSessionNameEmpty
	}

	snap, err := a.store.LoadSession(session)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	err = a.tmux.RestoreSession(snap)
	if err != nil && !errors.Is(err, tmux.ErrSessionExists) {
		return fmt.Errorf("restore session: %w", err)
	}

	err = a.store.MarkSessionAccessed(session, time.Now().UTC())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("mark session accessed: %w", err)
	}

	return nil
}

// handoffToTarget moves the user into the restored session. Inside tmux it hops
// the current client to the session. Outside tmux it attaches only when
// allowAttach is set — that path is reserved for the interactive picker, where
// the user picked a session and wants to land in it. Plain CLI commands
// (restore/bootstrap) pass allowAttach=false so they stay scriptable and don't
// hijack the terminal with a blocking attach.
func (a *App) handoffToTarget(target PickerTarget, allowAttach bool) error {
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

		return nil
	}

	// Outside tmux: attach only for the interactive picker; otherwise leave the
	// session restored-but-detached so scripts aren't blocked (#182/#183).
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
