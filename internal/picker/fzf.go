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

// Sentinel errors of the fzf-driven choosers.
var (
	errFzfTimedOut      = errors.New("fzf selection timed out")
	errNoSelection      = errors.New("no selection made")
	errInvalidFzfOutput = errors.New("invalid fzf output")
)

// fzfSelectionTimeout bounds how long a non-interactive --filter run may take.
const fzfSelectionTimeout = 30 * time.Second

// windowLineParts is the minimum tab-field count of a window fzf line: the
// visible display column plus the hidden session name and window index.
const windowLineParts = 3

// ErrNoSessions and ErrNoWindows are returned by the choosers when there is
// nothing to pick from: no saved sessions at all, or sessions without windows.
var (
	ErrNoSessions = errors.New("no sessions available")
	ErrNoWindows  = errors.New("no windows available")
)

// Upper bounds on how wide a single column may grow, so one very long session
// name or command can't blow the layout past the terminal. Longer values are
// truncated with an ellipsis (they still parse back via the hidden fields).
const (
	fzfNameMax = 40
	fzfWinName = 24
	fzfCmdMax  = 30
	// fzfColGap separates the visible, space-padded columns. A plain space run
	// (not a tab) so fzf renders it verbatim instead of snapping to tab stops.
	fzfColGap = "  "
)

// padCell right-pads s with spaces to the given rendered cell width, counting
// full-width glyphs correctly so unicode names still align. A string already at
// or over width is returned unchanged (callers clamp first).
func padCell(text string, width int) string {
	gap := width - ansi.StringWidth(text)
	if gap <= 0 {
		return text
	}

	return text + strings.Repeat(" ", gap)
}

// clampCell truncates s (ANSI-aware) to max rendered cells, appending an
// ellipsis when it overflows, so it can never push later columns out of line.
func clampCell(s string, maxWidth int) string {
	if ansi.StringWidth(s) <= maxWidth {
		return s
	}

	return ansi.Truncate(s, maxWidth, "…")
}

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

// sessionFZFLines renders one aligned line per session. The visible portion is a
// single, space-padded column (name, captured time, window count) so alignment
// never depends on fzf's tab-stop rendering (#200); the untruncated session name
// is appended as a hidden tab-delimited field for parsing the selection back.
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
		// Right-align the window count so single- and double-digit counts don't
		// leave ragged trailing widths.
		display := padCell(
			item.name,
			nameW,
		) + fzfColGap + item.when + fzfColGap + padLeft(
			item.count,
			countW,
		)
		// Hidden field: the real (unclamped) name, so a truncated display still
		// restores the right session.
		lines[idx] = display + "\t" + records[idx].SessionName
	}

	return lines
}

// ChooseSessionFZF presents a session-level fzf list (name, captured time,
// window count) and returns the name of the selected session. It returns
// ErrNoSessions when records is empty.
func ChooseSessionFZF(records []snapshot.Record) (string, error) {
	if len(records) == 0 {
		return "", ErrNoSessions
	}

	var input bytes.Buffer

	for _, line := range sessionFZFLines(records) {
		input.WriteString(line)
		input.WriteByte('\n')
	}

	// Only the first (visible) field is shown and searched; the trailing name
	// field is a hidden parse handle.
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

// windowFZFLines renders one fzf line per window across all sessions, with the
// windows of each session ordered by windowSort. The visible portion is a single
// space-padded column (session, index, name, command, captured time) so columns
// align independently of fzf's tab stops (#200). Two hidden tab-delimited fields
// follow — the session name and window index — which parseWindowSelection reads
// back into a Target.
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
			padLeft(strconv.Itoa(item.index), idxW), // numeric column, right-aligned
			padCell(item.name, nameW),
			padCell(item.cmd, cmdW),
			item.when,
		}, fzfColGap)

		// Hidden parse fields: real session name + window index.
		lines[idx] = fmt.Sprintf("%s\t%s\t%d", display, item.session, item.index)
	}

	return lines
}

// parseWindowSelection turns a window fzf line back into a Target pointing at the
// chosen session and window. The session name and index are the last two hidden
// tab-delimited fields (the first field is the aligned display column).
func parseWindowSelection(line string) (Target, error) {
	parts := strings.Split(line, "\t")
	// Display column + the two hidden parse fields (session, window index).
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

	// Only the aligned display column is shown/searched; the trailing session and
	// index fields are hidden parse handles.
	selected, err := runFZF(&input, "1")
	if err != nil {
		return Target{}, err
	}

	return parseWindowSelection(selected)
}

// padLeft left-pads s with spaces to width rendered cells, for right-aligning
// short numeric columns (e.g. the window index).
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
