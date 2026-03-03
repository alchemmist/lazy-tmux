package tmux

import (
	"bufio"
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

type Client struct {
	bin string
}

func NewClient(bin string) *Client {
	if strings.TrimSpace(bin) == "" {
		bin = "tmux"
	}
	return &Client{bin: bin}
}

func (c *Client) Run(args ...string) error {
	cmd := exec.Command(c.bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *Client) Output(args ...string) (string, error) {
	cmd := exec.Command(c.bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (c *Client) SessionExists(name string) bool {
	err := exec.Command(c.bin, "has-session", "-t", name).Run()
	return err == nil
}

func (c *Client) ListSessions() ([]string, error) {
	out, err := c.Output("list-sessions", "-F", "#{session_name}")
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

func (c *Client) CurrentSession() (string, error) {
	out, err := c.Output("display-message", "-p", "#S")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *Client) SocketPath() string {
	out, err := c.Output("display-message", "-p", "#{socket_path}")
	if err != nil {
		return "default"
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "default"
	}
	return v
}

func (c *Client) SwitchClient(session string) error {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	_, err := c.Output("switch-client", "-t", session)
	return err
}

func (c *Client) CaptureSession(name string) (snapshot.SessionSnapshot, error) {
	if !c.SessionExists(name) {
		return snapshot.SessionSnapshot{}, ErrSessionNotFound
	}

	metaOut, err := c.Output("display-message", "-p", "-t", name, "#{window_index}"+fieldSep+"#{pane_index}")
	if err != nil {
		return snapshot.SessionSnapshot{}, err
	}
	meta := strings.Split(strings.TrimSpace(metaOut), fieldSep)
	if len(meta) != 2 {
		return snapshot.SessionSnapshot{}, fmt.Errorf("unexpected session meta format: %q", strings.TrimSpace(metaOut))
	}
	currentWin, _ := strconv.Atoi(meta[0])
	currentPane, _ := strconv.Atoi(meta[1])

	wOut, err := c.Output("list-windows", "-t", name, "-F", "#{window_index}"+fieldSep+"#{window_name}"+fieldSep+"#{window_layout}"+fieldSep+"#{window_active}")
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
		w := snapshot.Window{
			Index:    idx,
			Name:     parts[1],
			Layout:   parts[2],
			IsActive: parts[3] == "1",
		}
		pOut, err := c.Output("list-panes", "-t", fmt.Sprintf("%s:%d", name, idx), "-F",
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
			p := strings.Split(pLine, fieldSep)
			if len(p) != 6 {
				continue
			}
			pIdx, _ := strconv.Atoi(p[0])
			panePID, _ := strconv.Atoi(strings.TrimSpace(p[4]))
			restoreCmd, _ := c.foregroundCommand(p[5], panePID)
			pane := snapshot.Pane{
				Index:       pIdx,
				CurrentPath: p[1],
				CurrentCmd:  p[2],
				IsActive:    p[3] == "1",
				RestoreCmd:  strings.TrimSpace(restoreCmd),
			}
			if pane.IsActive {
				w.ActivePane = pane.Index
			}
			w.Panes = append(w.Panes, pane)
		}
		sort.Slice(w.Panes, func(i, j int) bool { return w.Panes[i].Index < w.Panes[j].Index })
		windows = append(windows, w)
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

func (c *Client) RestoreSession(s snapshot.SessionSnapshot) error {
	if s.SessionName == "" {
		return errors.New("empty session name")
	}
	if c.SessionExists(s.SessionName) {
		return ErrSessionExists
	}
	if len(s.Windows) == 0 {
		return errors.New("session snapshot has no windows")
	}

	windows := make([]snapshot.Window, len(s.Windows))
	copy(windows, s.Windows)
	sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

	first := windows[0]
	if _, err := c.runWithShellFallback(newSessionArgs(s.SessionName, first), ""); err != nil {
		return err
	}

	// tmux creates the first window at server default index (often 0 or 1).
	// If snapshot index differs (e.g. sparse/non-renumbered windows), move it.
	if createdIdx, err := c.createdFirstWindowIndex(s.SessionName); err == nil && createdIdx != first.Index {
		_, err = c.Output(
			"move-window",
			"-s", fmt.Sprintf("%s:%d", s.SessionName, createdIdx),
			"-t", fmt.Sprintf("%s:%d", s.SessionName, first.Index),
		)
		if err != nil {
			return err
		}
	}

	if err := c.populateWindow(s.SessionName, first, first.Index); err != nil {
		return err
	}

	for i := 1; i < len(windows); i++ {
		w := windows[i]
		if err := c.createAndPopulateWindow(s.SessionName, w); err != nil {
			return err
		}
	}

	_, _ = c.Output("select-window", "-t", fmt.Sprintf("%s:%d", s.SessionName, s.CurrentWin))
	_, _ = c.Output("select-pane", "-t", fmt.Sprintf("%s:%d.%d", s.SessionName, s.CurrentWin, s.CurrentPane))
	return nil
}

func newSessionArgs(sessionName string, w snapshot.Window) []string {
	args := []string{"new-session", "-d", "-s", sessionName, "-n", w.Name}
	if path := firstPanePath(w); path != "" {
		args = append(args, "-c", path)
	}
	return args
}

func newWindowArgs(sessionName string, w snapshot.Window) []string {
	args := []string{"new-window", "-d", "-t", fmt.Sprintf("%s:%d", sessionName, w.Index), "-n", w.Name}
	if path := firstPanePath(w); path != "" {
		args = append(args, "-c", path)
	}
	return args
}

func (c *Client) createAndPopulateWindow(sessionName string, w snapshot.Window) error {
	if _, err := c.runWithShellFallback(newWindowArgs(sessionName, w), ""); err != nil {
		return err
	}
	return c.populateWindow(sessionName, w, w.Index)
}

func (c *Client) populateWindow(sessionName string, w snapshot.Window, windowIndex int) error {
	if err := c.ensurePaneCount(sessionName, w, windowIndex); err != nil {
		return err
	}
	c.restoreWindowScrollback(sessionName, w, windowIndex)
	if err := c.restoreWindowCommands(sessionName, w, windowIndex); err != nil {
		return err
	}
	if w.Layout != "" {
		_, _ = c.Output("select-layout", "-t", fmt.Sprintf("%s:%d", sessionName, windowIndex), w.Layout)
	}
	return nil
}

func (c *Client) CapturePaneScrollback(target string, lines int) (string, error) {
	if lines <= 0 {
		lines = 5000
	}
	return c.Output("capture-pane", "-p", "-e", "-S", fmt.Sprintf("-%d", lines), "-t", target)
}

func (c *Client) ensurePaneCount(sessionName string, w snapshot.Window, windowIndex int) error {
	if len(w.Panes) <= 1 {
		return nil
	}
	for i := 1; i < len(w.Panes); i++ {
		pane := w.Panes[i]
		args := []string{"split-window", "-d", "-t", fmt.Sprintf("%s:%d", sessionName, windowIndex)}
		if pane.CurrentPath != "" {
			args = append(args, "-c", pane.CurrentPath)
		}
		if _, err := c.runWithShellFallback(args, ""); err != nil {
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
		if home, err := os.UserHomeDir(); err == nil {
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
		if (cmd[0] == '"' && cmd[len(cmd)-1] == '"') || (cmd[0] == '\'' && cmd[len(cmd)-1] == '\'') {
			cmd = strings.TrimSpace(cmd[1 : len(cmd)-1])
		}
	}
	return cmd
}

func (c *Client) restoreWindowCommands(sessionName string, w snapshot.Window, windowIndex int) error {
	if len(w.Panes) == 0 {
		return nil
	}
	panes := make([]snapshot.Pane, len(w.Panes))
	copy(panes, w.Panes)
	sort.Slice(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })

	for _, pane := range panes {
		cmd := normalizedCommand(pane.RestoreCmd, pane.CurrentCmd)
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		target := fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, pane.Index)
		if _, err := c.Output("send-keys", "-t", target, cmd, "C-m"); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) restoreWindowScrollback(sessionName string, w snapshot.Window, windowIndex int) {
	if len(w.Panes) == 0 {
		return
	}
	panes := make([]snapshot.Pane, len(w.Panes))
	copy(panes, w.Panes)
	sort.Slice(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })

	for _, pane := range panes {
		if pane.Scrollback == nil || strings.TrimSpace(pane.Scrollback.Content) == "" {
			continue
		}
		target := fmt.Sprintf("%s:%d.%d", sessionName, windowIndex, pane.Index)
		tty, err := c.Output("display-message", "-p", "-t", target, "#{pane_tty}")
		if err != nil {
			continue
		}
		if err := writePaneTTY(strings.TrimSpace(tty), pane.Scrollback.Content); err != nil {
			continue
		}
	}
}

func writePaneTTY(path, content string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("empty tty path")
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, content)
	return err
}

func (c *Client) foregroundCommand(paneTTY string, panePID int) (string, error) {
	tty := strings.TrimSpace(paneTTY)
	if tty == "" {
		return "", nil
	}

	// "ps -t" may accept tty with or without "/dev/" prefix depending on platform.
	candidates := []string{tty}
	if b := strings.TrimPrefix(tty, "/dev/"); b != tty {
		candidates = append(candidates, b)
	}
	if b := filepath.Base(tty); b != tty {
		candidates = append(candidates, b)
	}

	var out []byte
	var err error
	for _, t := range candidates {
		cmd := exec.Command("ps", "-t", t, "-o", "pid=", "-o", "stat=", "-o", "command=")
		out, err = cmd.Output()
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", err
	}

	return pickForegroundCommand(splitLines(string(out)), panePID), nil
}

func pickForegroundCommand(lines []string, panePID int) string {
	fallback := ""
	for _, line := range lines {
		pid, stat, cmd, ok := parsePSLine(line)
		if !ok {
			continue
		}
		if pid == panePID || isShellCommand(cmd) {
			continue
		}
		if strings.Contains(stat, "+") {
			return cmd
		}
		if fallback == "" {
			fallback = cmd
		}
	}
	return fallback
}

func parsePSLine(line string) (int, string, string, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 3 {
		return 0, "", "", false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", "", false
	}
	stat := fields[1]
	cmd := strings.Join(fields[2:], " ")
	if strings.TrimSpace(cmd) == "" {
		return 0, "", "", false
	}
	return pid, stat, cmd, true
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

func (c *Client) runWithShellFallback(args []string, cmd string) (string, error) {
	out, err := c.Output(args...)
	if err == nil {
		return out, nil
	}

	// 1) Command failed to start; retry without explicit command to keep window/pane.
	withoutCmd := args
	if strings.TrimSpace(cmd) != "" && len(args) > 0 {
		withoutCmd = args[:len(args)-1]
		if out2, err2 := c.Output(withoutCmd...); err2 == nil {
			return out2, nil
		}
	}

	// 2) If directory is broken, retry without "-c <path>" too.
	minimal := stripOptionPair(withoutCmd, "-c")
	if len(minimal) > 0 {
		if out3, err3 := c.Output(minimal...); err3 == nil {
			return out3, nil
		}
	}

	return out, err
}

func stripOptionPair(args []string, opt string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == opt {
			i++ // skip option value
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func (c *Client) createdFirstWindowIndex(session string) (int, error) {
	out, err := c.Output("list-windows", "-t", session, "-F", "#{window_index}")
	if err != nil {
		return 0, err
	}
	lines := splitLines(out)
	if len(lines) == 0 {
		return 0, errors.New("no windows found after session creation")
	}
	return strconv.Atoi(lines[0])
}
