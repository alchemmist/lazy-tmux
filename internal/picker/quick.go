//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type quickPickerModel struct {
	sessions  []QuickSession
	theme     pickerTheme
	cursor    int
	width     int
	height    int
	selected  string
	cancelled bool
}

func newQuickPickerModel(sessions []QuickSession, themeName string) quickPickerModel {
	if themeName != themeLight {
		themeName = themeDark
	}

	cursor := 0
	for index := range sessions {
		if sessions[index].Current {
			cursor = index

			break
		}
	}

	return quickPickerModel{
		sessions:  sessions,
		theme:     newPickerThemeFor(themeName, colAccent),
		cursor:    cursor,
		width:     0,
		height:    0,
		selected:  "",
		cancelled: false,
	}
}

func (m quickPickerModel) Init() tea.Cmd {
	return nil
}

func (m quickPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case keyCtrlC, keyCtrlQ, keyEsc:
			m.cancelled = true

			return m, tea.Quit
		case keyCtrlJ, "j", "down":
			m = m.moved(1)
		case keyCtrlK, "k", "up":
			m = m.moved(-1)
		case keyEnter:
			if len(m.sessions) > 0 {
				m.selected = m.sessions[m.cursor].Name

				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m quickPickerModel) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 32
	}

	var buf strings.Builder
	buf.WriteString(m.theme.frameTop("sessions", strconv.Itoa(len(m.sessions)), width))
	buf.WriteString("\n")

	if len(m.sessions) == 0 {
		buf.WriteString(m.theme.frameLine(m.theme.faint.Render("No saved sessions"), width))
		buf.WriteString("\n")
	} else {
		start, end := m.visibleRange()
		for index := start; index < end; index++ {
			buf.WriteString(m.theme.frameLine(m.renderSession(index, width), width))
			buf.WriteString("\n")
		}
	}

	buf.WriteString(m.theme.frameBottom("^j/^k move  ·  enter switch  ·  esc quit", width))

	view := tea.NewView(buf.String())
	view.AltScreen = true

	return view
}

func (m quickPickerModel) moved(delta int) quickPickerModel {
	if len(m.sessions) == 0 {
		return m
	}

	m.cursor = (m.cursor + delta + len(m.sessions)) % len(m.sessions)

	return m
}

func (m quickPickerModel) visibleRange() (int, int) {
	available := max(1, m.height-2)
	if available >= len(m.sessions) {
		return 0, len(m.sessions)
	}

	start := m.cursor - available/2
	start = max(0, min(start, len(m.sessions)-available))

	return start, start + available
}

func (m quickPickerModel) renderSession(index, width int) string {
	session := m.sessions[index]
	state := "○"
	if session.Restored {
		state = "●"
	}

	name := session.Name
	if session.Current {
		name += "  current"
	}

	line := "  " + state + " " + name
	line = clampWidth(line, max(0, width-frameChromeWidth))
	if index == m.cursor {
		line = m.theme.selBar.Width(max(0, width-frameChromeWidth)).Render(line)
	}

	return line
}

//nolint:gochecknoglobals
var newQuickPickerRunner = func(m quickPickerModel) pickerRunner {
	return tea.NewProgram(m)
}

func ChooseQuickSession(sessions []QuickSession, themeName string) (string, error) {
	if tuiDisabled() {
		return "", errTUIDisabled
	}

	runner := newQuickPickerRunner(newQuickPickerModel(sessions, themeName))
	finalModel, err := runner.Run()
	if err != nil {
		return "", fmt.Errorf("run quick picker: %w", err)
	}

	result, ok := finalModel.(quickPickerModel)
	if !ok {
		return "", errUnexpectedModel
	}
	if result.cancelled {
		return "", errSelectionCanceled
	}
	if result.selected == "" {
		return "", errNoSessionSelected
	}

	return result.selected, nil
}
