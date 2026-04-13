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
	"sort"
	"strconv"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const fieldSep = "\x1f"

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
	bin    string
	runner commandRunner
}

func NewClient(bin string) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "tmux"
	}

	return &Client{bin: bin, runner: execRunner{}}
}

func NewClientWithRunner(bin string, cmdRunner commandRunner) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "tmux"
	}

	tmuxClient := &Client{bin: bin}
	if cmdRunner != nil {
		tmuxClient.runner = cmdRunner
	} else {
		tmuxClient.runner = execRunner{}
	}

	return tmuxClient
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

	metaOut, err := client.Output(
		"display-message",
		"-p",
		"-t",
		sessionTarget(name),
		"#{window_index}"+fieldSep+"#{pane_index}",
	)
	if err != nil {
		return snapshot.SessionSnapshot{}, err
	}

	meta := strings.Split(strings.TrimSpace(metaOut), fieldSep)
	if len(meta) != 2 {
		return snapshot.SessionSnapshot{}, fmt.Errorf(
			"unexpected session meta format: %q",
			strings.TrimSpace(metaOut),
		)
	}

	currentWin, _ := strconv.Atoi(meta[0])
	currentPane, _ := strconv.Atoi(meta[1])

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
		parts := strings.Split(line, fieldSep)
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
			parts := strings.Split(pLine, fieldSep)
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

	return snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		CapturedAt:  nowUTC(),
		CurrentWin:  currentWin,
		CurrentPane: currentPane,
		Windows:     windows,
	}, nil
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

	_, err = client.Output(
		"select-window",
		"-t",
		sessionWindowTarget(sessionSnapshot.SessionName, sessionSnapshot.CurrentWin),
	)
	if err != nil {
		return fmt.Errorf("select window: %w", err)
	}

	_, err = client.Output(
		"select-pane",
		"-t",
		sessionPaneTarget(
			sessionSnapshot.SessionName,
			sessionSnapshot.CurrentWin,
			sessionSnapshot.CurrentPane,
		),
	)
	if err != nil {
		return fmt.Errorf("select pane: %w", err)
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

		target := sessionPaneTarget(sessionName, windowIndex, pane.Index)

		_, err := client.Output("send-keys", "-t", target, cmd, "C-m")
		if err != nil {
			return err
		}
	}

	return nil
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
