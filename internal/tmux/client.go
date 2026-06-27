package tmux

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const fieldSep = "\x1f"

// defaultRestoreSettleTimeout bounds how long RestoreSession waits for restored
// pane commands to actually start before returning. restoreSettlePollInterval
// is how often it re-checks pane state while waiting.
const (
	defaultRestoreSettleTimeout = 5 * time.Second
	restoreSettlePollInterval   = 100 * time.Millisecond
)

// splitFields splits a tmux -F output line on the field separator. tmux 3.5a
// (and likely other versions) escapes control bytes in format output as octal
// (so the 0x1f separator arrives as the literal four characters `\037`),
// whereas tmux 3.6+ emits the raw byte. Normalize the escaped form back to the
// separator so capture works across tmux versions.
func splitFields(line string) []string {
	line = strings.ReplaceAll(line, `\037`, fieldSep)
	return strings.Split(line, fieldSep)
}

var (
	ErrSessionNotFound = errors.New("tmux session not found")
	ErrSessionExists   = errors.New("tmux session already exists")
)

var paneTTYWriter = writePaneTTY

type commandResult struct {
	stdout string
	err    error
}

type commandRunner interface {
	runCommand(args ...string) commandResult
}

type execRunner struct{}

func (execRunner) runCommand(args ...string) commandResult {
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return commandResult{"", fmt.Errorf(
			"tmux %s: %w (%s)",
			strings.Join(args[1:], " "),
			err,
			strings.TrimSpace(string(out)),
		)}
	}

	return commandResult{string(out), nil}
}

type Client struct {
	bin           string
	runner        commandRunner
	settleTimeout time.Duration

	// allowlist limits which commands RestoreSession replays, keyed by
	// executable name. allowlistSet distinguishes "no allowlist configured"
	// (restore everything) from a configured-but-empty allowlist (restore
	// nothing).
	allowlist    map[string]struct{}
	allowlistSet bool
}

func NewClient(bin string) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "tmux"
	}

	return &Client{bin: bin, runner: execRunner{}, settleTimeout: defaultRestoreSettleTimeout}
}

func NewClientWithRunner(bin string, cmdRunner commandRunner) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "tmux"
	}

	tmuxClient := &Client{bin: bin, settleTimeout: defaultRestoreSettleTimeout}
	if cmdRunner != nil {
		tmuxClient.runner = cmdRunner
	} else {
		tmuxClient.runner = execRunner{}
	}

	return tmuxClient
}

// SetRestoreTimeout bounds how long RestoreSession waits for restored pane
// commands to start before returning. A value <= 0 disables waiting, restoring
// the previous fire-and-forget behavior.
func (client *Client) SetRestoreTimeout(timeout time.Duration) {
	client.settleTimeout = timeout
}

// SetRestoreAllowlist configures which commands RestoreSession may replay,
// matched by executable name. A nil slice clears the allowlist so every command
// is restored; a non-nil slice (including an empty one) activates it, restoring
// only the listed commands. List entries are normalized to their executable
// name, so both "nvim" and "/usr/bin/nvim" match a pane running nvim.
func (client *Client) SetRestoreAllowlist(list []string) {
	if list == nil {
		client.allowlistSet = false
		client.allowlist = nil

		return
	}

	client.allowlistSet = true
	client.allowlist = make(map[string]struct{}, len(list))

	for _, item := range list {
		name := executableName(strings.TrimSpace(item))
		if name != "" {
			client.allowlist[name] = struct{}{}
		}
	}
}

func sessionTarget(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "=") {
		return name
	}

	return "=" + name
}

func sessionWindowTarget(name string, windowIndex int) string {
	return fmt.Sprintf("%s:%d", sessionTarget(name), windowIndex)
}

func sessionPaneTarget(name string, windowIndex, paneIndex int) string {
	return fmt.Sprintf("%s:%d.%d", sessionTarget(name), windowIndex, paneIndex)
}

func PaneTarget(name string, windowIndex, paneIndex int) string {
	return sessionPaneTarget(name, windowIndex, paneIndex)
}

func sessionWindowBaseTarget(name string) string {
	return sessionTarget(name) + ":"
}

func (client *Client) Run(args ...string) error {
	cmd := exec.CommandContext(context.Background(), client.bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("run tmux: %w", err)
	}

	return nil
}

func (client *Client) Output(args ...string) (string, error) {
	allArgs := append([]string{client.bin}, args...)
	res := client.runner.runCommand(allArgs...)

	return res.stdout, res.err
}

func (client *Client) SessionExists(name string) bool {
	_, err := client.Output("has-session", "-t", sessionTarget(name))
	return err == nil
}

func (client *Client) ListSessions() ([]string, error) {
	out, err := client.Output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}

		return nil, err
	}

	lines := splitLines(out)
	sort.Strings(lines)

	return lines, nil
}

func (client *Client) CurrentSession() (string, error) {
	out, err := client.Output("display-message", "-p", "#S")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

func (client *Client) SocketPath() string {
	out, err := client.Output("display-message", "-p", "#{socket_path}")
	if err != nil {
		return "default"
	}

	v := strings.TrimSpace(out)
	if v == "" {
		return "default"
	}

	return v
}

func (client *Client) SwitchClient(session string) error {
	if os.Getenv("TMUX") == "" {
		return nil
	}

	_, err := client.Output("switch-client", "-t", sessionTarget(session))

	return err
}

func (client *Client) KillWindow(session string, windowIndex int) error {
	_, err := client.Output("kill-window", "-t", sessionWindowTarget(session, windowIndex))
	return err
}

func (client *Client) KillSession(session string) error {
	_, err := client.Output("kill-session", "-t", sessionTarget(session))
	return err
}

func (client *Client) RenameWindow(session string, windowIndex int, name string) error {
	_, err := client.Output("rename-window", "-t", sessionWindowTarget(session, windowIndex), name)
	return err
}

func (client *Client) RenameSession(session, name string) error {
	_, err := client.Output("rename-session", "-t", sessionTarget(session), name)
	return err
}

func (client *Client) NewSession(name string) error {
	_, err := client.Output("new-session", "-d", "-s", name)
	return err
}

func (client *Client) NewWindow(session, name string) error {
	args := []string{"new-window", "-d", "-t", sessionWindowBaseTarget(session)}
	if strings.TrimSpace(name) != "" {
		args = append(args, "-n", name)
	}

	_, err := client.Output(args...)

	return err
}

func (client *Client) CaptureSession(name string) (snapshot.SessionSnapshot, error) {
	if !client.SessionExists(name) {
		return snapshot.SessionSnapshot{}, ErrSessionNotFound
	}

	wOut, err := client.Output(
		"list-windows",
		"-t",
		sessionTarget(name),
		"-F",
		"#{window_index}"+fieldSep+"#{window_name}"+fieldSep+"#{window_layout}"+fieldSep+"#{window_active}",
	)
	if err != nil {
		return snapshot.SessionSnapshot{}, err
	}

	windows := make([]snapshot.Window, 0)

	for _, line := range splitLines(wOut) {
		parts := splitFields(line)
		if len(parts) != 4 {
			continue
		}

		idx, _ := strconv.Atoi(parts[0])
		window := snapshot.Window{
			Index:    idx,
			Name:     parts[1],
			Layout:   parts[2],
			IsActive: parts[3] == "1",
		}

		pOut, err := client.Output("list-panes", "-t", sessionWindowTarget(name, idx), "-F",
			"#{pane_index}"+fieldSep+
				"#{pane_current_path}"+fieldSep+
				"#{pane_current_command}"+fieldSep+
				"#{pane_active}"+fieldSep+
				"#{pane_pid}"+fieldSep+
				"#{pane_tty}",
		)
		if err != nil {
			return snapshot.SessionSnapshot{}, err
		}

		for _, pLine := range splitLines(pOut) {
			parts := splitFields(pLine)
			if len(parts) != 6 {
				continue
			}

			pIdx, _ := strconv.Atoi(parts[0])
			panePID, _ := strconv.Atoi(strings.TrimSpace(parts[4]))
			restoreCmd, _ := client.foregroundCommand(parts[5], panePID)

			pane := snapshot.Pane{
				Index:       pIdx,
				CurrentPath: parts[1],
				CurrentCmd:  parts[2],
				IsActive:    parts[3] == "1",
				RestoreCmd:  strings.TrimSpace(restoreCmd),
			}
			if pane.IsActive {
				window.ActivePane = pane.Index
			}

			window.Panes = append(window.Panes, pane)
		}

		sort.Slice(
			window.Panes,
			func(i, j int) bool { return window.Panes[i].Index < window.Panes[j].Index },
		)

		windows = append(windows, window)
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

	// Derive the current window/pane from the active window/pane we just
	// enumerated. This is authoritative and works for detached, clientless
	// sessions, unlike "display-message -t <session>", whose window/pane format
	// variables are empty without an attached client on some tmux versions.
	currentWin, currentPane := currentWindowPane(windows)

	return snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CapturedAt:  nowUTC(),
		CurrentWin:  currentWin,
		CurrentPane: currentPane,
		Windows:     windows,
	}, nil
}

// currentWindowPane reports the index of the active window and its active pane.
// It falls back to the first window when no window is flagged active.
func currentWindowPane(windows []snapshot.Window) (int, int) {
	for _, window := range windows {
		if window.IsActive {
			return window.Index, window.ActivePane
		}
	}

	if len(windows) > 0 {
		return windows[0].Index, windows[0].ActivePane
	}

	return 0, 0
}

// findWindow returns the window with the given index, if present.
func findWindow(windows []snapshot.Window, index int) (snapshot.Window, bool) {
	for _, window := range windows {
		if window.Index == index {
			return window, true
		}
	}

	return snapshot.Window{}, false
}

// paneExists reports whether a pane with the given index is present.
func paneExists(panes []snapshot.Pane, index int) bool {
	for _, pane := range panes {
		if pane.Index == index {
			return true
		}
	}

	return false
}

// clampPane resolves a pane index to one that actually exists in the window,
// preferring the requested index, then the window's active pane, then its
// first pane.
func clampPane(window snapshot.Window, paneIndex int) int {
	if paneExists(window.Panes, paneIndex) {
		return paneIndex
	}

	if paneExists(window.Panes, window.ActivePane) {
		return window.ActivePane
	}

	if len(window.Panes) > 0 {
		return window.Panes[0].Index
	}

	return paneIndex
}

// resolveRestoreFocus picks the window/pane to focus after a restore, tolerating
// snapshots whose recorded current window/pane no longer matches the restored
// windows. Snapshots captured before the base-index fix store current_window 0
// while windows are indexed from 1 (e.g. under `set -g base-index 1`), so
// selecting the recorded index would fail with "can't find window: 0".
func resolveRestoreFocus(
	sessionSnapshot snapshot.SessionSnapshot,
	windows []snapshot.Window,
) (int, int) {
	// Honor the recorded focus when its window still exists.
	if window, ok := findWindow(windows, sessionSnapshot.CurrentWin); ok {
		return window.Index, clampPane(window, sessionSnapshot.CurrentPane)
	}

	// Otherwise fall back to the active window (or the first one).
	win, pane := currentWindowPane(windows)
	if window, ok := findWindow(windows, win); ok {
		return win, clampPane(window, pane)
	}

	return win, pane
}

func (client *Client) RestoreSession(sessionSnapshot snapshot.SessionSnapshot) error {
	if sessionSnapshot.SessionName == "" {
		return errors.New("empty session name")
	}

	if client.SessionExists(sessionSnapshot.SessionName) {
		return ErrSessionExists
	}

	if len(sessionSnapshot.Windows) == 0 {
		return errors.New("session snapshot has no windows")
	}

	windows := make([]snapshot.Window, len(sessionSnapshot.Windows))
	copy(windows, sessionSnapshot.Windows)
	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

	first := windows[0]

	_, err := client.runWithShellFallback(
		newSessionArgs(sessionSnapshot.SessionName, first),
		"",
	)
	if err != nil {
		return err
	}

	createdIdx, err := client.createdFirstWindowIndex(
		sessionSnapshot.SessionName,
	)
	if err != nil {
		return err
	}

	if createdIdx != first.Index {
		_, err = client.Output(
			"move-window",
			"-s", sessionWindowTarget(sessionSnapshot.SessionName, createdIdx),
			"-t", sessionWindowTarget(sessionSnapshot.SessionName, first.Index),
		)
		if err != nil {
			return err
		}
	}

	err = client.populateWindow(sessionSnapshot.SessionName, first, first.Index)
	if err != nil {
		return err
	}

	for i := 1; i < len(windows); i++ {
		w := windows[i]

		err := client.createAndPopulateWindow(sessionSnapshot.SessionName, w)
		if err != nil {
			return err
		}
	}

	// Focus a window/pane that actually exists in the restored session. The
	// recorded indices come from the snapshot and may not match what the
	// restoring server created — when base-index / pane-base-index differ
	// between save and restore, the recorded window 0 / pane 0 need not exist —
	// so resolve against the live layout. Focus is cosmetic: a select failure
	// must never abort an otherwise-successful restore, which is how `bootstrap`
	// used to die with "can't find window: 0".
	win, pane, ok := client.resolveLiveFocus(
		sessionSnapshot.SessionName,
		sessionSnapshot,
		windows,
	)
	if ok {
		_, _ = client.Output(
			"select-window",
			"-t",
			sessionWindowTarget(sessionSnapshot.SessionName, win),
		)
		_, _ = client.Output(
			"select-pane",
			"-t",
			sessionPaneTarget(sessionSnapshot.SessionName, win, pane),
		)
	}

	client.waitForRestoredCommands(sessionSnapshot.SessionName, windows)

	return nil
}

func newSessionArgs(sessionName string, w snapshot.Window) []string {
	args := []string{"new-session", "-d", "-s", sessionName, "-n", w.Name}
	if path := firstPanePath(w); path != "" {
		args = append(args, "-c", path)
	}

	return args
}

func newWindowArgs(sessionName string, win snapshot.Window) []string {
	args := []string{
		"new-window",
		"-d",
		"-t",
		sessionWindowTarget(sessionName, win.Index),
		"-n",
		win.Name,
	}
	if path := firstPanePath(win); path != "" {
		args = append(args, "-c", path)
	}

	return args
}

func (client *Client) CapturePaneScrollback(target string, lines int) (string, error) {
	if lines <= 0 {
		lines = 5000
	}

	return client.Output("capture-pane", "-p", "-e", "-S", fmt.Sprintf("-%d", lines), "-t", target)
}

func (client *Client) createAndPopulateWindow(sessionName string, win snapshot.Window) error {
	_, err := client.runWithShellFallback(newWindowArgs(sessionName, win), "")
	if err != nil {
		return err
	}

	return client.populateWindow(sessionName, win, win.Index)
}

func (client *Client) populateWindow(
	sessionName string,
	window snapshot.Window,
	windowIndex int,
) error {
	err := client.ensurePaneCount(sessionName, window, windowIndex)
	if err != nil {
		return err
	}

	client.restoreWindowScrollback(sessionName, window, windowIndex)

	err = client.restoreWindowCommands(sessionName, window, windowIndex)
	if err != nil {
		return err
	}

	if window.Layout != "" {
		_, _ = client.Output(
			"select-layout",
			"-t",
			sessionWindowTarget(sessionName, windowIndex),
			window.Layout,
		)
	}

	return nil
}

func (client *Client) ensurePaneCount(
	sessionName string,
	window snapshot.Window,
	windowIndex int,
) error {
	if len(window.Panes) <= 1 {
		return nil
	}

	for i := 1; i < len(window.Panes); i++ {
		pane := window.Panes[i]
		args := []string{"split-window", "-d", "-t", sessionWindowTarget(sessionName, windowIndex)}

		if pane.CurrentPath != "" {
			args = append(args, "-c", pane.CurrentPath)
		}

		_, err := client.runWithShellFallback(args, "")
		if err != nil {
			return err
		}
	}

	return nil
}

func firstPanePath(w snapshot.Window) string {
	if len(w.Panes) == 0 {
		return ""
	}

	pane := w.Panes[0]

	path := pane.CurrentPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = home
		}
	}

	return filepath.Clean(path)
}

func normalizedCommand(restore, current string) string {
	restore = sanitizeCommand(restore)
	if restore != "" && !isShellCommand(restore) {
		return restore
	}

	current = sanitizeCommand(current)
	if current != "" && !isShellCommand(current) {
		return current
	}

	return ""
}

func isShellCommand(cmd string) bool {
	base := executableName(cmd)
	shells := map[string]struct{}{
		"bash": {},
		"zsh":  {},
		"fish": {},
		"sh":   {},
		"ksh":  {},
	}
	_, ok := shells[base]

	return ok
}

func executableName(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}

	base := filepath.Base(fields[0])

	return strings.TrimPrefix(base, "-")
}

func sanitizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if len(cmd) >= 2 {
		if (cmd[0] == '"' && cmd[len(cmd)-1] == '"') ||
			(cmd[0] == '\'' && cmd[len(cmd)-1] == '\'') {
			cmd = strings.TrimSpace(cmd[1 : len(cmd)-1])
		}
	}

	return cmd
}

func (client *Client) restoreWindowCommands(
	sessionName string,
	window snapshot.Window,
	windowIndex int,
) error {
	if len(window.Panes) == 0 {
		return nil
	}

	panes := make([]snapshot.Pane, len(window.Panes))
	copy(panes, window.Panes)
	sort.Slice(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })

	for _, pane := range panes {
		cmd := normalizedCommand(pane.RestoreCmd, pane.CurrentCmd)
		if strings.TrimSpace(cmd) == "" {
			continue
		}

		// Skip commands the allowlist does not permit, so lazy-tmux never
		// replays arbitrary programs the user has not opted into (issue #74).
		if !client.commandAllowed(executableName(cmd)) {
			continue
		}

		target := sessionPaneTarget(sessionName, windowIndex, pane.Index)

		_, err := client.Output("send-keys", "-t", target, cmd, "C-m")
		if err != nil {
			return err
		}
	}

	return nil
}

// commandAllowed reports whether a command with the given executable name may be
// restored under the current allowlist. With no allowlist configured every
// command is allowed.
func (client *Client) commandAllowed(executable string) bool {
	if !client.allowlistSet {
		return true
	}

	_, ok := client.allowlist[executable]

	return ok
}

// expectedPaneCommands maps "window.pane" to the foreground command we expect a
// pane to be running once its restore command has actually started. Panes with
// nothing to restore, or whose command the allowlist blocks, are omitted so the
// settle wait does not block on commands that will never start.
func (client *Client) expectedPaneCommands(windows []snapshot.Window) map[string]string {
	want := make(map[string]string)

	for _, window := range windows {
		for _, pane := range window.Panes {
			exe := executableName(normalizedCommand(pane.RestoreCmd, pane.CurrentCmd))
			if exe == "" || !client.commandAllowed(exe) {
				continue
			}

			want[fmt.Sprintf("%d.%d", window.Index, pane.Index)] = exe
		}
	}

	return want
}

// paneCommands reports the current foreground command of every pane in the
// session, keyed by "window.pane". Errors yield an empty map so callers can
// simply retry.
func (client *Client) paneCommands(sessionName string) map[string]string {
	result := make(map[string]string)

	out, err := client.Output(
		"list-panes", "-s", "-t", sessionTarget(sessionName),
		"-F", "#{window_index} #{pane_index} #{pane_current_command}",
	)
	if err != nil {
		return result
	}

	for _, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		result[fields[0]+"."+fields[1]] = fields[2]
	}

	return result
}

// waitForRestoredCommands blocks until every pane that had a command to restore
// is actually running it, or until the settle timeout elapses. Without this the
// restore returns while panes are still at the shell prompt, so automation could
// not tell whether the session was fully restored when the command exited (see
// issue #106). It is best-effort: once the deadline passes it returns regardless.
//
// Tradeoff: a restored command that exits very quickly leaves its pane back at
// the shell, so its expected foreground command is never observed and that pane
// holds the wait until settleTimeout. This is acceptable because snapshots
// capture the *foreground* command of a pane, which in practice is a long-lived
// program (editor, REPL, pager, server) rather than a fast one-shot; the timeout
// bounds the worst case and a value of 0 opts out of waiting entirely.
func (client *Client) waitForRestoredCommands(sessionName string, windows []snapshot.Window) {
	if client.settleTimeout <= 0 {
		return
	}

	want := client.expectedPaneCommands(windows)
	if len(want) == 0 {
		return
	}

	deadline := time.Now().Add(client.settleTimeout)

	for {
		got := client.paneCommands(sessionName)

		pending := false

		for key, exe := range want {
			if got[key] != exe {
				pending = true

				break
			}
		}

		if !pending || time.Now().After(deadline) {
			return
		}

		time.Sleep(restoreSettlePollInterval)
	}
}

func (client *Client) restoreWindowScrollback(
	sessionName string,
	window snapshot.Window,
	windowIndex int,
) {
	if len(window.Panes) == 0 {
		return
	}

	panes := make([]snapshot.Pane, len(window.Panes))
	copy(panes, window.Panes)
	sort.Slice(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })

	for _, pane := range panes {
		if pane.Scrollback == nil || strings.TrimSpace(pane.Scrollback.Content) == "" {
			continue
		}

		target := sessionPaneTarget(sessionName, windowIndex, pane.Index)

		tty, err := client.Output("display-message", "-p", "-t", target, "#{pane_tty}")
		if err != nil {
			continue
		}

		err = paneTTYWriter(strings.TrimSpace(tty), pane.Scrollback.Content)
		if err != nil {
			continue
		}
	}
}

func writePaneTTY(path, content string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("empty tty path")
	}

	if !strings.HasPrefix(path, "/dev/pts/") && !strings.HasPrefix(path, "/dev/tty") {
		return fmt.Errorf("unsupported tty path: %s", path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat tty: %w", err)
	}

	if fi.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("tty path is not a character device: %s", path)
	}

	ttyFile, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open tty: %w", err)
	}

	defer func() { _ = ttyFile.Close() }()

	_, err = io.WriteString(ttyFile, content)
	if err != nil {
		return fmt.Errorf("write to tty: %w", err)
	}

	return nil
}

func (client *Client) foregroundCommand(paneTTY string, panePID int) (string, error) {
	tty := strings.TrimSpace(paneTTY)
	if tty == "" {
		return "", nil
	}

	// "ps -t" may accept tty with or without "/dev/" prefix depending on platform.
	candidates := []string{tty}
	if b, ok := strings.CutPrefix(tty, "/dev/"); ok {
		candidates = append(candidates, b)
	}

	if b := filepath.Base(tty); b != tty {
		candidates = append(candidates, b)
	}

	var out []byte

	var err error

	for _, t := range candidates {
		cmd := exec.CommandContext(
			context.Background(),
			"ps",
			"-t",
			t,
			"-o",
			"pid=",
			"-o",
			"ppid=",
			"-o",
			"stat=",
			"-o",
			"command=",
		)

		out, err = cmd.Output()
		if err == nil {
			break
		}
	}

	if err != nil {
		return "", fmt.Errorf("get output: %w", err)
	}

	return pickForegroundCommand(splitLines(string(out)), panePID), nil
}

type psProcess struct {
	pid  int
	ppid int
	stat string
	cmd  string
}

func pickForegroundCommand(lines []string, panePID int) string {
	// First pass: collect all non-shell processes
	allProcesses := make([]psProcess, 0, len(lines))

	for _, line := range lines {
		pid, ppid, stat, cmd, ok := parsePSLine(line)
		if !ok {
			continue
		}

		// Skip the shell process itself
		if pid == panePID {
			continue
		}

		// Skip shell commands
		if isShellCommand(cmd) {
			continue
		}

		allProcesses = append(allProcesses, psProcess{
			pid:  pid,
			ppid: ppid,
			stat: stat,
			cmd:  cmd,
		})
	}

	if len(allProcesses) == 0 {
		return ""
	}

	// Build set of ALL process PIDs for parent lookup
	allPIDs := make(map[int]struct{}, len(allProcesses))
	for _, p := range allProcesses {
		allPIDs[p.pid] = struct{}{}
	}

	// Find root processes: those whose parent is NOT in our process set
	// (meaning parent is either the shell or some other process outside this tty)
	var rootProcesses []psProcess

	for _, p := range allProcesses {
		if _, parentInSet := allPIDs[p.ppid]; !parentInSet {
			rootProcesses = append(rootProcesses, p)
		}
	}

	// If we found root processes, prefer foreground ones
	if len(rootProcesses) > 0 {
		for _, p := range rootProcesses {
			if strings.Contains(p.stat, "+") {
				return p.cmd
			}
		}
		// Fallback: return first root process
		return rootProcesses[0].cmd
	}

	// If no root processes found, fallback to first foreground process
	for _, p := range allProcesses {
		if strings.Contains(p.stat, "+") {
			return p.cmd
		}
	}

	// Last fallback: return first process
	if len(allProcesses) > 0 {
		return allProcesses[0].cmd
	}

	return ""
}

func parsePSLine(line string) (int, int, string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 4 {
		return 0, 0, "", "", false
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, "", "", false
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, "", "", false
	}

	stat := fields[2]

	cmd := strings.Join(fields[3:], " ")
	if strings.TrimSpace(cmd) == "" {
		return 0, 0, "", "", false
	}

	return pid, ppid, stat, cmd, true
}

func splitLines(in string) []string {
	s := bufio.NewScanner(strings.NewReader(in))
	out := make([]string, 0)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		out = append(out, line)
	}

	return out
}

func (client *Client) runWithShellFallback(args []string, cmd string) (string, error) {
	out, err := client.Output(args...)
	if err == nil {
		return out, nil
	}

	// 1) Command failed to start; retry without explicit command to keep window/pane.
	withoutCmd := args
	if strings.TrimSpace(cmd) != "" && len(args) > 0 {
		withoutCmd = args[:len(args)-1]

		out2, err2 := client.Output(withoutCmd...)
		if err2 == nil {
			return out2, nil
		}
	}

	// 2) If directory is broken, retry without "-c <path>" too.
	minimal := stripOptionPair(withoutCmd, "-c")
	if len(minimal) > 0 {
		out3, err3 := client.Output(minimal...)
		if err3 == nil {
			return out3, nil
		}
	}

	return out, err
}

func stripOptionPair(args []string, opt string) []string {
	out := make([]string, 0, len(args))

	for idx := 0; idx < len(args); idx++ {
		if args[idx] == opt {
			idx++ // skip option value
			continue
		}

		out = append(out, args[idx])
	}

	return out
}

// resolveLiveFocus picks a window/pane to focus that actually exists in the
// just-restored session. It starts from the snapshot's recorded focus (via
// resolveRestoreFocus) but falls back to the first live window/pane whenever the
// recorded index is absent — which happens when the restoring server's
// base-index / pane-base-index differ from the saved snapshot. The bool is false
// when the session has no live windows to focus.
func (client *Client) resolveLiveFocus(
	session string,
	sessionSnapshot snapshot.SessionSnapshot,
	windows []snapshot.Window,
) (int, int, bool) {
	liveWindows := client.liveIndexes("list-windows", sessionTarget(session), "#{window_index}")
	if len(liveWindows) == 0 {
		return 0, 0, false
	}

	wantWin, wantPane := resolveRestoreFocus(sessionSnapshot, windows)

	win := wantWin
	if !slices.Contains(liveWindows, win) {
		win = liveWindows[0]
	}

	pane := wantPane
	livePanes := client.liveIndexes(
		"list-panes",
		sessionWindowTarget(session, win),
		"#{pane_index}",
	)
	if len(livePanes) > 0 && !slices.Contains(livePanes, pane) {
		pane = livePanes[0]
	}

	return win, pane, true
}

// liveIndexes runs a tmux list command and parses the integer index it formats,
// one per line. It returns nil on any error so callers can fall back gracefully.
func (client *Client) liveIndexes(listCmd, target, format string) []int {
	out, err := client.Output(listCmd, "-t", target, "-F", format)
	if err != nil {
		return nil
	}

	var indexes []int
	for _, line := range splitLines(out) {
		n, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil {
			continue
		}

		indexes = append(indexes, n)
	}

	return indexes
}

func (client *Client) createdFirstWindowIndex(session string) (int, error) {
	out, err := client.Output("list-windows", "-t", sessionTarget(session), "-F", "#{window_index}")
	if err != nil {
		return 0, err
	}

	lines := splitLines(out)
	if len(lines) == 0 {
		return 0, errors.New("no windows found after session creation")
	}

	idx, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, fmt.Errorf("parse window index: %w", err)
	}

	return idx, nil
}
