package tmux

import (
	"bufio"
	"errors"
	"fmt"
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
		pOut, err := c.Output("list-panes", "-t", fmt.Sprintf("%s:%d", name, idx), "-F", "#{pane_index}"+fieldSep+"#{pane_current_path}"+fieldSep+"#{pane_current_command}"+fieldSep+"#{pane_active}"+fieldSep+"#{pane_start_command}")
		if err != nil {
			return snapshot.SessionSnapshot{}, err
		}
		for _, pLine := range splitLines(pOut) {
			p := strings.Split(pLine, fieldSep)
			if len(p) != 5 {
				continue
			}
			pIdx, _ := strconv.Atoi(p[0])
			pane := snapshot.Pane{
				Index:        pIdx,
				CurrentPath:  p[1],
				CurrentCmd:   p[2],
				IsActive:     p[3] == "1",
				StartCommand: p[4],
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
	firstPath, firstCmd := firstPaneInit(first)
	args := []string{"new-session", "-d", "-s", s.SessionName, "-n", first.Name}
	if firstPath != "" {
		args = append(args, "-c", firstPath)
	}
	if firstCmd != "" {
		args = append(args, firstCmd)
	}
	if _, err := c.Output(args...); err != nil {
		return err
	}
	if err := c.ensurePaneCount(s.SessionName, first, first.Index); err != nil {
		return err
	}
	if first.Layout != "" {
		_, _ = c.Output("select-layout", "-t", fmt.Sprintf("%s:%d", s.SessionName, first.Index), first.Layout)
	}

	for i := 1; i < len(windows); i++ {
		w := windows[i]
		path, cmd := firstPaneInit(w)
		wArgs := []string{"new-window", "-d", "-t", fmt.Sprintf("%s:%d", s.SessionName, w.Index), "-n", w.Name}
		if path != "" {
			wArgs = append(wArgs, "-c", path)
		}
		if cmd != "" {
			wArgs = append(wArgs, cmd)
		}
		if _, err := c.Output(wArgs...); err != nil {
			return err
		}
		if err := c.ensurePaneCount(s.SessionName, w, w.Index); err != nil {
			return err
		}
		if w.Layout != "" {
			_, _ = c.Output("select-layout", "-t", fmt.Sprintf("%s:%d", s.SessionName, w.Index), w.Layout)
		}
	}

	_, _ = c.Output("select-window", "-t", fmt.Sprintf("%s:%d", s.SessionName, s.CurrentWin))
	_, _ = c.Output("select-pane", "-t", fmt.Sprintf("%s:%d.%d", s.SessionName, s.CurrentWin, s.CurrentPane))
	return nil
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
		if cmd := normalizedStartCommand(pane.StartCommand, pane.CurrentCmd); cmd != "" {
			args = append(args, cmd)
		}
		if _, err := c.Output(args...); err != nil {
			return err
		}
	}
	return nil
}

func firstPaneInit(w snapshot.Window) (string, string) {
	if len(w.Panes) == 0 {
		return "", ""
	}
	pane := w.Panes[0]
	path := pane.CurrentPath
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	}
	return filepath.Clean(path), normalizedStartCommand(pane.StartCommand, pane.CurrentCmd)
}

func normalizedStartCommand(start, current string) string {
	start = strings.TrimSpace(start)
	if start == "" {
		return ""
	}
	if isShellCommand(start) {
		return ""
	}
	if strings.TrimSpace(current) == "" {
		return start
	}
	return start
}

func isShellCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	cmd = strings.TrimPrefix(cmd, "-")
	base := filepath.Base(cmd)
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
