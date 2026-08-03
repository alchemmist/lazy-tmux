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
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/charmbracelet/x/term"
)

var (
	errEmptySessionName     = errors.New("empty session name")
	errSnapshotHasNoWindows = errors.New("session snapshot has no windows")
	errEmptyTTYPath         = errors.New("empty tty path")
	errUnsupportedTTYPath   = errors.New("unsupported tty path")
	errTTYNotCharDevice     = errors.New("tty path is not a character device")
	errNoWindowsAfterCreate = errors.New("no windows found after session creation")
)

const fieldSep = "|"

const (
	defaultRestoreSettleTimeout = 5 * time.Second
	restoreSettlePollInterval   = 100 * time.Millisecond
)

func splitFieldsN(line string, n int) []string {
	return strings.SplitN(line, fieldSep, n)
}

var (
	ErrSessionNotFound = errors.New("tmux session not found")
	ErrSessionExists   = errors.New("tmux session already exists")
)

//nolint:gochecknoglobals // test seam: tests capture pane-tty writes
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
	// #nosec G204 -- argv[0] is the tmux binary, user-configured on purpose (--tmux-bin)
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

	allowlist    []*regexp.Regexp
	allowlistSet bool

	denylist []*regexp.Regexp

	resolver RestoreCommandResolver
}

type RestoreCommandResolver interface {
	Resolve(pane snapshot.Pane) string
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

func (client *Client) SetRestoreTimeout(timeout time.Duration) {
	client.settleTimeout = timeout
}

func (client *Client) SetRestoreAllowlist(list []string) {
	if list == nil {
		client.allowlistSet = false
		client.allowlist = nil

		return
	}

	client.allowlistSet = true
	client.allowlist = compileCommandPatterns(list)
}

func (client *Client) SetRestoreDenylist(list []string) {
	if len(list) == 0 {
		client.denylist = nil

		return
	}

	client.denylist = compileCommandPatterns(list)
}

func compileCommandPatterns(list []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(list))

	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		re, err := regexp.Compile("^(?:" + item + ")$")
		if err != nil {
			continue
		}

		patterns = append(patterns, re)
	}

	return patterns
}

func (client *Client) SetRestoreResolver(resolver RestoreCommandResolver) {
	client.resolver = resolver
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
	// #nosec G204 -- client.bin is the tmux binary, user-configured on purpose (--tmux-bin)
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
		if isNoServerError(err) {
			return nil, nil
		}

		return nil, err
	}

	lines := splitLines(out)
	sort.Strings(lines)

	return lines, nil
}

func isNoServerError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "no server running"):
		return true
	case strings.Contains(msg, "error connecting to") &&
		strings.Contains(msg, "no such file or directory"):
		return true
	default:
		return false
	}
}

func (client *Client) SessionsLastAttached() map[string]time.Time {
	out, err := client.Output(
		"list-sessions",
		"-F",
		"#{session_last_attached}"+fieldSep+"#{session_name}",
	)
	if err != nil {
		return nil
	}

	result := make(map[string]time.Time)

	for _, line := range splitLines(out) {
		fields := splitFieldsN(line, 2)
		if len(fields) != 2 {
			continue
		}

		name := fields[1]
		if strings.TrimSpace(name) == "" {
			continue
		}

		secs, parseErr := strconv.ParseInt(strings.TrimSpace(fields[0]), 10, 64)
		if parseErr != nil || secs <= 0 {
			continue
		}

		result[name] = time.Unix(secs, 0).UTC()
	}

	return result
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

func (client *Client) InsideTmux() bool {
	return strings.TrimSpace(os.Getenv("TMUX")) != ""
}

//nolint:gochecknoglobals // test seam, see comment above
var attachExec = syscall.Exec

//nolint:gochecknoglobals // test seam, see comment above
var hasControllingTTY = func() bool {
	return term.IsTerminal(os.Stdout.Fd())
}

func (client *Client) AttachSession(target string) error {
	if !hasControllingTTY() {
		return nil
	}

	bin, err := exec.LookPath(client.bin)
	if err != nil {
		return fmt.Errorf("locate tmux %q: %w", client.bin, err)
	}

	args := []string{bin, "attach-session", "-t", sessionTarget(target)}

	return attachExec(bin, args, os.Environ())
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
		"#{window_index}"+fieldSep+"#{window_layout}"+fieldSep+"#{window_active}"+fieldSep+"#{window_name}",
	)
	if err != nil {
		return snapshot.SessionSnapshot{}, err
	}

	windows := make([]snapshot.Window, 0)

	for _, line := range splitLines(wOut) {
		parts := splitFieldsN(line, windowLineFields)
		if len(parts) != windowLineFields {
			continue
		}

		idx, _ := strconv.Atoi(parts[0])
		window := snapshot.Window{
			Index:      idx,
			Layout:     parts[1],
			IsActive:   parts[2] == "1",
			Name:       parts[3],
			ActivePane: 0,
			Panes:      nil,
		}

		err = client.capturePanes(name, &window)
		if err != nil {
			return snapshot.SessionSnapshot{}, err
		}

		windows = append(windows, window)
	}

	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

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

func findWindow(windows []snapshot.Window, index int) (snapshot.Window, bool) {
	for _, window := range windows {
		if window.Index == index {
			return window, true
		}
	}

	return snapshot.Window{}, false
}

func paneExists(panes []snapshot.Pane, index int) bool {
	for _, pane := range panes {
		if pane.Index == index {
			return true
		}
	}

	return false
}

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

func resolveRestoreFocus(
	sessionSnapshot snapshot.SessionSnapshot,
	windows []snapshot.Window,
) (int, int) {
	if window, ok := findWindow(windows, sessionSnapshot.CurrentWin); ok {
		return window.Index, clampPane(window, sessionSnapshot.CurrentPane)
	}

	win, pane := currentWindowPane(windows)
	if window, ok := findWindow(windows, win); ok {
		return win, clampPane(window, pane)
	}

	return win, pane
}

func (client *Client) RestoreSession(
	ctx context.Context,
	sessionSnapshot snapshot.SessionSnapshot,
) error {
	if sessionSnapshot.SessionName == "" {
		return errEmptySessionName
	}

	if client.SessionExists(sessionSnapshot.SessionName) {
		return ErrSessionExists
	}

	if len(sessionSnapshot.Windows) == 0 {
		return errSnapshotHasNoWindows
	}

	windows := make([]snapshot.Window, len(sessionSnapshot.Windows))
	copy(windows, sessionSnapshot.Windows)
	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

	err := client.restoreFirstWindow(sessionSnapshot.SessionName, windows[0])
	if err != nil {
		return err
	}

	for idx := 1; idx < len(windows); idx++ {
		err = restoreCanceled(ctx)
		if err != nil {
			return err
		}

		err = client.createAndPopulateWindow(sessionSnapshot.SessionName, windows[idx])
		if err != nil {
			return err
		}
	}

	err = restoreCanceled(ctx)
	if err != nil {
		return err
	}

	client.selectRestoredFocus(sessionSnapshot, windows)

	client.waitForRestoredCommands(ctx, sessionSnapshot.SessionName, windows)

	return nil
}

func restoreCanceled(ctx context.Context) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("restore canceled: %w", err)
	}

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

	return client.Output(
		"capture-pane",
		"-p",
		"-e",
		"-J",
		"-S",
		fmt.Sprintf("-%d", lines),
		"-t",
		target,
	)
}

func (client *Client) createAndPopulateWindow(sessionName string, win snapshot.Window) error {
	err := client.runWithShellFallback(newWindowArgs(sessionName, win), "")
	if err != nil {
		return err
	}

	return client.populateWindow(sessionName, win, win.Index)
}

func (client *Client) selectRestoredFocus(
	sessionSnapshot snapshot.SessionSnapshot,
	windows []snapshot.Window,
) {
	win, pane, ok := client.resolveLiveFocus(
		sessionSnapshot.SessionName,
		sessionSnapshot,
		windows,
	)
	if !ok {
		return
	}

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

func (client *Client) restoreFirstWindow(sessionName string, first snapshot.Window) error {
	err := client.runWithShellFallback(
		newSessionArgs(sessionName, first),
		"",
	)
	if err != nil {
		return err
	}

	createdIdx, err := client.createdFirstWindowIndex(sessionName)
	if err != nil {
		return err
	}

	if createdIdx != first.Index {
		_, err = client.Output(
			"move-window",
			"-s", sessionWindowTarget(sessionName, createdIdx),
			"-t", sessionWindowTarget(sessionName, first.Index),
		)
		if err != nil {
			return err
		}
	}

	return client.populateWindow(sessionName, first, first.Index)
}

func (client *Client) capturePanes(name string, window *snapshot.Window) error {
	pOut, err := client.Output("list-panes", "-t", sessionWindowTarget(name, window.Index), "-F",
		"#{pane_index}"+fieldSep+
			"#{pane_active}"+fieldSep+
			"#{pane_pid}"+fieldSep+
			"#{pane_tty}"+fieldSep+
			"#{pane_current_command}"+fieldSep+
			"#{pane_current_path}",
	)
	if err != nil {
		return err
	}

	for _, pLine := range splitLines(pOut) {
		parts := splitFieldsN(pLine, paneLineFields)
		if len(parts) != paneLineFields {
			continue
		}

		pIdx, _ := strconv.Atoi(parts[0])
		panePID, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		restoreCmd, _ := client.foregroundCommand(parts[3], panePID)

		pane := snapshot.Pane{
			Index:       pIdx,
			IsActive:    parts[1] == "1",
			CurrentCmd:  parts[4],
			CurrentPath: parts[5],
			RestoreCmd:  strings.TrimSpace(restoreCmd),
			Scrollback:  nil,
			Meta:        nil,
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

	return nil
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

		err := client.runWithShellFallback(args, "")
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

func (client *Client) effectiveRestoreCommand(pane snapshot.Pane) string {
	if client.resolver != nil {
		if override := strings.TrimSpace(client.resolver.Resolve(pane)); override != "" {
			return override
		}
	}

	return normalizedCommand(pane.RestoreCmd, pane.CurrentCmd)
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

	return restore
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
		cmd := client.effectiveRestoreCommand(pane)
		if strings.TrimSpace(cmd) == "" {
			continue
		}

		if !client.commandAllowed(cmd) {
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

func (client *Client) commandAllowed(command string) bool {
	command = strings.TrimSpace(command)

	if matchesAny(client.denylist, command) {
		return false
	}

	if !client.allowlistSet {
		return true
	}

	return matchesAny(client.allowlist, command)
}

func matchesAny(patterns []*regexp.Regexp, command string) bool {
	for _, re := range patterns {
		if re.MatchString(command) {
			return true
		}
	}

	return false
}

func (client *Client) expectedPaneCommands(windows []snapshot.Window) map[string]string {
	want := make(map[string]string)

	for _, window := range windows {
		for _, pane := range window.Panes {
			cmd := client.effectiveRestoreCommand(pane)

			exe := executableName(cmd)
			if exe == "" || !client.commandAllowed(cmd) {
				continue
			}

			want[fmt.Sprintf("%d.%d", window.Index, pane.Index)] = exe
		}
	}

	return want
}

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
		if len(fields) < paneCommandFields {
			continue
		}

		result[fields[0]+"."+fields[1]] = fields[2]
	}

	return result
}

func (client *Client) waitForRestoredCommands(
	ctx context.Context,
	sessionName string,
	windows []snapshot.Window,
) {
	if client.settleTimeout <= 0 {
		return
	}

	want := client.expectedPaneCommands(windows)
	if len(want) == 0 {
		return
	}

	deadline := time.Now().Add(client.settleTimeout)

	for {
		if ctx.Err() != nil {
			return
		}

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

		select {
		case <-ctx.Done():
			return
		case <-time.After(restoreSettlePollInterval):
		}
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
		return errEmptyTTYPath
	}

	if !strings.HasPrefix(path, "/dev/pts/") && !strings.HasPrefix(path, "/dev/tty") {
		return fmt.Errorf("%w: %s", errUnsupportedTTYPath, path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat tty: %w", err)
	}

	if fi.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("%w: %s", errTTYNotCharDevice, path)
	}

	ttyFile, err := os.OpenFile(
		path,
		os.O_WRONLY,
		0,
	) // #nosec G304 -- validated above: /dev/* character device
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
		cmd := exec.CommandContext( // #nosec G204 -- fixed "ps" binary, variable args are pids/format flags
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

	command := pickForegroundCommand(splitLines(string(out)), panePID)
	if command != "" {
		return command, nil
	}

	// Some terminal wrappers give the child process a different tty from the
	// shell tmux reports for the pane. Fall back to the process tree so tools
	// such as Codex are still detected instead of being saved as zsh.
	cmd := exec.CommandContext( // #nosec G204 -- fixed "ps" binary and numeric pane PID
		context.Background(),
		"ps",
		"-ax",
		"-o",
		"pid=",
		"-o",
		"ppid=",
		"-o",
		"stat=",
		"-o",
		"command=",
	)
	allOut, allErr := cmd.Output()
	if allErr != nil {
		return "", nil
	}

	return pickForegroundCommand(processTreeLines(splitLines(string(allOut)), panePID), panePID), nil
}

func processTreeLines(lines []string, rootPID int) []string {
	processes := make(map[int]psProcess)
	for _, line := range lines {
		pid, ppid, stat, command, ok := parsePSLine(line)
		if ok {
			processes[pid] = psProcess{pid: pid, ppid: ppid, stat: stat, cmd: command}
		}
	}

	children := make(map[int][]int)
	for _, process := range processes {
		children[process.ppid] = append(children[process.ppid], process.pid)
	}

	seen := map[int]bool{rootPID: true}
	queue := []int{rootPID}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			if seen[child] {
				continue
			}
			seen[child] = true
			queue = append(queue, child)
		}
	}

	tree := make([]string, 0, len(seen)-1)
	for pid := range seen {
		if pid == rootPID {
			continue
		}
		process, ok := processes[pid]
		if !ok {
			continue
		}
		tree = append(tree, fmt.Sprintf("%d %d %s %s", process.pid, process.ppid, process.stat, process.cmd))
	}

	return tree
}

type psProcess struct {
	pid  int
	ppid int
	stat string
	cmd  string
}

func collectCandidateProcesses(lines []string, panePID int) ([]psProcess, []psProcess) {
	nonShell := make([]psProcess, 0, len(lines))

	var shells []psProcess

	for _, line := range lines {
		pid, ppid, stat, cmd, ok := parsePSLine(line)
		if !ok {
			continue
		}

		if pid == panePID {
			continue
		}

		process := psProcess{
			pid:  pid,
			ppid: ppid,
			stat: stat,
			cmd:  cmd,
		}

		if isShellCommand(cmd) {
			if strings.Contains(stat, "+") {
				shells = append(shells, process)
			}

			continue
		}

		nonShell = append(nonShell, process)
	}

	return nonShell, shells
}

func pickForegroundCommand(lines []string, panePID int) string {
	nonShell, shells := collectCandidateProcesses(lines, panePID)

	if cmd := pickFromCandidates(nonShell); cmd != "" {
		return cmd
	}

	return pickFromCandidates(shells)
}

func pickFromCandidates(allProcesses []psProcess) string {
	if len(allProcesses) == 0 {
		return ""
	}

	allPIDs := make(map[int]struct{}, len(allProcesses))
	for _, p := range allProcesses {
		allPIDs[p.pid] = struct{}{}
	}

	var rootProcesses []psProcess

	for _, p := range allProcesses {
		if _, parentInSet := allPIDs[p.ppid]; !parentInSet {
			rootProcesses = append(rootProcesses, p)
		}
	}

	if len(rootProcesses) > 0 {
		for _, p := range rootProcesses {
			if strings.Contains(p.stat, "+") {
				return p.cmd
			}
		}

		return rootProcesses[0].cmd
	}

	for _, p := range allProcesses {
		if strings.Contains(p.stat, "+") {
			return p.cmd
		}
	}

	if len(allProcesses) > 0 {
		return allProcesses[0].cmd
	}

	return ""
}

const (
	windowLineFields  = 4
	paneLineFields    = 6
	paneCommandFields = 3
	psLineFields      = 4
)

func parsePSLine(line string) (int, int, string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < psLineFields {
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

func (client *Client) runWithShellFallback(args []string, cmd string) error {
	_, err := client.Output(args...)
	if err == nil {
		return nil
	}

	withoutCmd := args
	if strings.TrimSpace(cmd) != "" && len(args) > 0 {
		withoutCmd = args[:len(args)-1]

		_, err2 := client.Output(withoutCmd...)
		if err2 == nil {
			return nil
		}
	}

	minimal := stripOptionPair(withoutCmd, "-c")
	if len(minimal) > 0 {
		_, err3 := client.Output(minimal...)
		if err3 == nil {
			return nil
		}
	}

	return err
}

func stripOptionPair(args []string, opt string) []string {
	out := make([]string, 0, len(args))

	for idx := 0; idx < len(args); idx++ {
		if args[idx] == opt {
			idx++

			continue
		}

		out = append(out, args[idx])
	}

	return out
}

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
		return 0, errNoWindowsAfterCreate
	}

	idx, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, fmt.Errorf("parse window index: %w", err)
	}

	return idx, nil
}
