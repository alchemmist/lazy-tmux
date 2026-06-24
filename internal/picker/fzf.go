package picker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var ErrNoSessions = errors.New("no sessions available")

// runFZF pipes input into fzf and returns the first selected line. With a TTY it
// runs interactively; without one it uses --filter "" so all lines pass through
// and the first is taken (used by tests and non-interactive callers).
func runFZF(input *bytes.Buffer, withNth string) (string, error) {
	args := []string{
		"fzf",
		"--delimiter", "\t",
		"--with-nth", withNth,
	}

	if isTerminal(os.Stdout) {
		args = append(args,
			"--prompt", "lazy-tmux> ",
			"--height", "100%",
			"--layout", "reverse",
		)
	} else {
		args = append(args, "--filter", "")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = input

	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("fzf selection timed out")
		}

		return "", fmt.Errorf("fzf selection canceled or failed: %w", err)
	}

	// In --filter mode fzf prints every matching line; take the first one.
	selected := strings.TrimSpace(string(out))
	selected = strings.SplitN(selected, "\n", 2)[0]

	if strings.TrimSpace(selected) == "" {
		return "", fmt.Errorf("no selection made")
	}

	return selected, nil
}

func ChooseSessionFZF(records []snapshot.Record) (string, error) {
	if len(records) == 0 {
		return "", ErrNoSessions
	}

	var input bytes.Buffer

	for _, r := range records {
		line := fmt.Sprintf(
			"%s\t%s\t%dw\n",
			r.SessionName,
			r.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			r.Windows,
		)
		input.WriteString(line)
	}

	selected, err := runFZF(&input, "1,2,3")
	if err != nil {
		return "", err
	}

	parts := strings.Split(selected, "\t")
	if strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("invalid fzf output")
	}

	return parts[0], nil
}

// windowFZFLines renders one fzf line per window across all sessions, with the
// windows of each session ordered by windowSort. Each line is tab-delimited:
// session, window index, window name, command, captured time. Fields 0-1 are
// parsed back into a Target by parseWindowSelection; the rest are for display
// and fuzzy matching.
func windowFZFLines(sessions []Session, windowSort []WindowSortKey) []string {
	var lines []string

	for _, session := range sessions {
		windows := make([]snapshot.Window, len(session.Windows))
		copy(windows, session.Windows)
		sortWindows(windows, windowSort)

		captured := session.Record.CapturedAt.Local().Format("2006-01-02 15:04:05")

		for _, window := range windows {
			name := window.Name
			if name == "" {
				name = "-"
			}

			cmd := windowPreviewCommand(window)
			if cmd == "" {
				cmd = "-"
			}

			lines = append(lines, fmt.Sprintf(
				"%s\t%d\t%s\t%s\t%s",
				session.Record.SessionName,
				window.Index,
				name,
				cmd,
				captured,
			))
		}
	}

	return lines
}

// parseWindowSelection turns a window fzf line back into a Target pointing at the
// chosen session and window.
func parseWindowSelection(line string) (Target, error) {
	parts := strings.Split(line, "\t")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return Target{}, fmt.Errorf("invalid fzf output")
	}

	index, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return Target{}, fmt.Errorf("invalid window index in fzf output: %w", err)
	}

	return Target{SessionName: parts[0], WindowIndex: &index}, nil
}

// ChooseWindowFZF presents a flat, window-level fzf list (one line per window)
// and returns the selected window as a Target. Selecting a window restores its
// session focused on that window.
func ChooseWindowFZF(sessions []Session, windowSort []WindowSortKey) (Target, error) {
	if len(sessions) == 0 {
		return Target{}, ErrNoSessions
	}

	lines := windowFZFLines(sessions, windowSort)
	if len(lines) == 0 {
		return Target{}, ErrNoSessions
	}

	var input bytes.Buffer
	for _, line := range lines {
		input.WriteString(line)
		input.WriteByte('\n')
	}

	selected, err := runFZF(&input, "1,2,3,4,5")
	if err != nil {
		return Target{}, err
	}

	return parseWindowSelection(selected)
}
