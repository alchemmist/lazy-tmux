//go:build !lazy_fzf

package picker

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The loading view ports the landing page's generative ASCII field
// (docs/src/components/AsciiField.tsx) into the terminal: several drifting sine
// waves interfere into a breathing field of glyphs, sparse-to-dense along the
// ramp, with the brand amber lighting up the peaks. It fills the whole popup
// while a picked session restores, so a slow restore shows motion instead of a
// black screen (#199).
const (
	animFPS   = 30
	animRamp  = " .·:-=+*#%@"
	animGrace = 120 * time.Millisecond // don't flash the field on a fast restore
)

// frameMsg advances the animation; graceMsg reveals it after the grace period;
// restoreDoneMsg quits once the background restore has finished.
type (
	frameMsg       struct{}
	graceMsg       struct{}
	restoreDoneMsg struct{}
)

type loadingModel struct {
	name    string
	done    <-chan struct{}
	width   int
	height  int
	frame   int
	started bool // grace elapsed → render the field (else stay invisible)

	dim   lipgloss.Style
	mid   lipgloss.Style
	hot   lipgloss.Style
	label lipgloss.Style
}

func newLoadingModel(sessionName string, done <-chan struct{}) loadingModel {
	return loadingModel{
		name:  strings.TrimSpace(sessionName),
		done:  done,
		dim:   lipgloss.NewStyle().Foreground(lipgloss.Color(colFaint)),
		mid:   lipgloss.NewStyle().Foreground(lipgloss.Color(colText)),
		hot:   lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)),
		label: lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true),
	}
}

func (m loadingModel) Init() tea.Cmd {
	return tea.Batch(
		waitForRestore(m.done),
		tea.Tick(animGrace, func(time.Time) tea.Msg { return graceMsg{} }),
	)
}

// waitForRestore blocks in tea's goroutine until the restore signals completion.
func waitForRestore(done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-done
		return restoreDoneMsg{}
	}
}

func frameTick() tea.Cmd {
	return tea.Tick(time.Second/animFPS, func(time.Time) tea.Msg { return frameMsg{} })
}

func (m loadingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil
	case graceMsg:
		m.started = true

		return m, frameTick()
	case frameMsg:
		m.frame++

		return m, frameTick()
	case restoreDoneMsg:
		return m, tea.Quit
	case tea.KeyPressMsg:
		// Let the user bail out; the restore still finishes in the background.
		if s := msg.String(); s == "ctrl+c" || s == "esc" || s == "q" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m loadingModel) View() tea.View {
	// Before the grace period elapses we render nothing (and stay off the alt
	// screen) so a restore that finishes quickly never flashes the animation.
	if !m.started {
		return tea.NewView("")
	}

	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}

	if height <= 0 {
		height = 24
	}

	view := tea.NewView(m.field(width, height))
	view.AltScreen = true

	return view
}

// field renders one animation frame: a width×height grid of ramp glyphs from the
// interfering-sine field, with a centered "restoring <name>…" caption.
func (m loadingModel) field(width, height int) string {
	phase := float64(m.frame) / animFPS
	cols := float64(width)
	rows := float64(height)

	caption := "restoring…"
	if m.name != "" {
		caption = "restoring " + m.name + "…"
	}

	captionRow := height / 2
	capStart := max(0, (width-len([]rune(caption)))/2)

	var buf strings.Builder

	for row := range height {
		// The caption owns its row: a clean band keeps it readable over the field.
		if row == captionRow {
			buf.WriteString(strings.Repeat(" ", capStart) + m.label.Render(caption))
		} else {
			m.writeFieldRow(&buf, row, width, cols, rows, phase)
		}

		if row < height-1 {
			buf.WriteByte('\n')
		}
	}

	return buf.String()
}

// writeFieldRow renders one field row, grouping runs of same-styled glyphs so
// each frame emits few ANSI escapes.
func (m loadingModel) writeFieldRow(
	buf *strings.Builder,
	row, width int,
	cols, rows, phase float64,
) {
	var (
		run   strings.Builder
		tier  = -1
		flush = func(next int) {
			if run.Len() > 0 {
				buf.WriteString(m.styleFor(tier).Render(run.String()))
				run.Reset()
			}

			tier = next
		}
	)

	for col := range width {
		glyph, level := fieldGlyph(float64(col), float64(row), cols, rows, phase)
		if level != tier {
			flush(level)
		}

		run.WriteRune(glyph)
	}

	flush(-1)
}

func (m loadingModel) styleFor(tier int) lipgloss.Style {
	switch tier {
	case 2:
		return m.hot
	case 1:
		return m.mid
	default:
		return m.dim
	}
}

// fieldGlyph computes the ramp glyph and color tier (0 dim, 1 mid, 2 hot) for the
// cell at (col,row), mirroring the landing page's sine-interference math. Short
// names (x/y/v) match that formula.
//
//nolint:varnamelen // math formula reads best with x/y/v
func fieldGlyph(col, row, cols, rows, phase float64) (rune, int) {
	x := col * 0.18
	y := row * 0.28

	v := math.Sin(x+phase*0.6) +
		math.Sin(y*0.8-phase*0.5) +
		math.Sin((x+y)*0.5+phase*0.4) +
		math.Sin(math.Hypot(x-cols*0.09, y-rows*0.14)-phase*0.8)
	v /= 4
	v = math.Max(0, math.Min(1, (v+1)/2))

	ramp := []rune(animRamp)
	glyph := ramp[int(v*float64(len(ramp)-1))]

	switch {
	case glyph == ' ':
		return ' ', 0
	case v > 0.86:
		return glyph, 2
	case v > 0.5:
		return glyph, 1
	default:
		return glyph, 0
	}
}

// RunRestoreAnimation shows the loading field until done is closed. Without a
// controlling terminal (tests, piped output) it is a no-op and the caller waits
// on done itself.
func RunRestoreAnimation(sessionName string, done <-chan struct{}) error {
	if !isTerminal(os.Stdout) {
		return nil
	}

	_, err := tea.NewProgram(newLoadingModel(sessionName, done)).Run()
	if err != nil {
		return fmt.Errorf("run restore animation: %w", err)
	}

	return nil
}
