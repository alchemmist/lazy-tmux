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

// Sentinel errors of the fzf-driven choosers.
var (
	errFzfTimedOut      = errors.New("fzf selection timed out")
	errNoSelection      = errors.New("no selection made")
	errInvalidFzfOutput = errors.New("invalid fzf output")
)

// fzfSelectionTimeout bounds how long a non-interactive --filter run may take.
const fzfSelectionTimeout = 30 * time.Second

// ErrNoSessions and ErrNoWindows are returned by the choosers when there is
// nothing to pick from: no saved sessions at all, or sessions without windows.
var (
	ErrNoSessions = errors.New("no sessions available")
	ErrNoWindows  = errors.New("no windows available")
)

// runFZF pipes input into fzf and returns the first selected line. With a TTY it
// runs interactively; without one it uses --filter "" so all lines pass through
// and the first is taken (used by tests and non-interactive callers).
func runFZF(input *bytes.Buffer, withNth string) (string, error) {
	args := []string{
		"fzf",
		"--delimiter", "\t",
		"--with-nth", withNth,
	}

	interactive := isTerminal(os.Stdout)
	if interactive {
		args = append(args,
			"--prompt", "lazy-tmux> ",
			"--height", "100%",
			"--layout", "reverse",
		)
	} else {
		args = append(args, "--filter", "")
	}

	// Only non-interactive --filter runs are bounded: a user browsing the
	// interactive picker must never have fzf killed under them.
	ctx := context.Background()

	if !interactive {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(context.Background(), fzfSelectionTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(
		ctx,
		args[0],
		args[1:]...) // #nosec G204 -- argv[0] is the fzf binary with args built above
	cmd.Stdin = input

	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errFzfTimedOut
		}

		return "", fmt.Errorf("fzf selection canceled or failed: %w", err)
	}

	// In --filter mode fzf prints every matching line; take the first one.
	selected := strings.TrimSpace(string(out))
	selected = strings.SplitN(selected, "\n", 2)[0]

	if strings.TrimSpace(selected) == "" {
		return "", errNoSelection
	}

	return selected, nil
}

// ChooseSessionFZF presents a session-level fzf list (name, captured time,
// window count) and returns the name of the selected session. It returns
// ErrNoSessions when records is empty.
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
		return "", errInvalidFzfOutput
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
		return Target{}, errInvalidFzfOutput
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
		// Sessions exist but none of them have any windows to pick from.
		return Target{}, ErrNoWindows
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
