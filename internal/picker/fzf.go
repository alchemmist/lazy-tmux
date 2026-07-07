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
	"github.com/charmbracelet/x/ansi"
)

var (
	errFzfTimedOut      = errors.New("fzf selection timed out")
	errNoSelection      = errors.New("no selection made")
	errInvalidFzfOutput = errors.New("invalid fzf output")
)

const fzfSelectionTimeout = 30 * time.Second

const windowLineParts = 3

var (
	ErrNoSessions = errors.New("no sessions available")
	ErrNoWindows  = errors.New("no windows available")
)

const (
	fzfNameMax = 40
	fzfWinName = 24
	fzfCmdMax  = 30
	fzfColGap  = "  "
)

func padCell(text string, width int) string {
	gap := width - ansi.StringWidth(text)
	if gap <= 0 {
		return text
	}

	return text + strings.Repeat(" ", gap)
}

func clampCell(s string, maxWidth int) string {
	if ansi.StringWidth(s) <= maxWidth {
		return s
	}

	return ansi.Truncate(s, maxWidth, "…")
}

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

	selected := strings.TrimSpace(string(out))
	selected = strings.SplitN(selected, "\n", 2)[0]

	if strings.TrimSpace(selected) == "" {
		return "", errNoSelection
	}

	return selected, nil
}

func sessionFZFLines(records []snapshot.Record) []string {
	type row struct{ name, when, count string }

	rows := make([]row, len(records))

	var nameW, countW int

	for idx, r := range records {
		name := clampCell(r.SessionName, fzfNameMax)
		rows[idx] = row{
			name:  name,
			when:  r.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			count: fmt.Sprintf("%dw", r.Windows),
		}

		nameW = maxInt(nameW, ansi.StringWidth(name))
		countW = maxInt(countW, ansi.StringWidth(rows[idx].count))
	}

	lines := make([]string, len(records))
	for idx, item := range rows {
		display := padCell(
			item.name,
			nameW,
		) + fzfColGap + item.when + fzfColGap + padLeft(
			item.count,
			countW,
		)
		lines[idx] = display + "\t" + records[idx].SessionName
	}

	return lines
}

func ChooseSessionFZF(records []snapshot.Record) (string, error) {
	if len(records) == 0 {
		return "", ErrNoSessions
	}

	var input bytes.Buffer

	for _, line := range sessionFZFLines(records) {
		input.WriteString(line)
		input.WriteByte('\n')
	}

	selected, err := runFZF(&input, "1")
	if err != nil {
		return "", err
	}

	parts := strings.Split(selected, "\t")

	name := strings.TrimSpace(parts[len(parts)-1])
	if name == "" {
		return "", errInvalidFzfOutput
	}

	return name, nil
}

func windowFZFLines(sessions []Session, windowSort []WindowSortKey) []string {
	type row struct {
		session, name, cmd, when string
		index                    int
	}

	var rows []row

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

			rows = append(rows, row{
				session: session.Record.SessionName,
				index:   window.Index,
				name:    clampCell(name, fzfWinName),
				cmd:     clampCell(cmd, fzfCmdMax),
				when:    captured,
			})
		}
	}

	var sessW, idxW, nameW, cmdW int

	for _, r := range rows {
		sessW = maxInt(sessW, ansi.StringWidth(clampCell(r.session, fzfNameMax)))
		idxW = maxInt(idxW, len(strconv.Itoa(r.index)))
		nameW = maxInt(nameW, ansi.StringWidth(r.name))
		cmdW = maxInt(cmdW, ansi.StringWidth(r.cmd))
	}

	lines := make([]string, len(rows))

	for idx, item := range rows {
		display := strings.Join([]string{
			padCell(clampCell(item.session, fzfNameMax), sessW),
			padLeft(strconv.Itoa(item.index), idxW),
			padCell(item.name, nameW),
			padCell(item.cmd, cmdW),
			item.when,
		}, fzfColGap)

		lines[idx] = fmt.Sprintf("%s\t%s\t%d", display, item.session, item.index)
	}

	return lines
}

func parseWindowSelection(line string) (Target, error) {
	parts := strings.Split(line, "\t")
	if len(parts) < windowLineParts {
		return Target{}, errInvalidFzfOutput
	}

	session := parts[len(parts)-2]
	if strings.TrimSpace(session) == "" {
		return Target{}, errInvalidFzfOutput
	}

	index, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil {
		return Target{}, fmt.Errorf("invalid window index in fzf output: %w", err)
	}

	return Target{SessionName: session, WindowIndex: &index}, nil
}

func ChooseWindowFZF(sessions []Session, windowSort []WindowSortKey) (Target, error) {
	if len(sessions) == 0 {
		return Target{}, ErrNoSessions
	}

	lines := windowFZFLines(sessions, windowSort)
	if len(lines) == 0 {
		return Target{}, ErrNoWindows
	}

	var input bytes.Buffer
	for _, line := range lines {
		input.WriteString(line)
		input.WriteByte('\n')
	}

	selected, err := runFZF(&input, "1")
	if err != nil {
		return Target{}, err
	}

	return parseWindowSelection(selected)
}

func padLeft(text string, width int) string {
	gap := width - ansi.StringWidth(text)
	if gap <= 0 {
		return text
	}

	return strings.Repeat(" ", gap) + text
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}
