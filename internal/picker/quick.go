//go:build !lazy_fzf

package picker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type quickPickerModel struct {
	sessions            []QuickSession
	visible             []QuickSession
	queryInput          textinput.Model
	theme               pickerTheme
	cursor              int
	width               int
	height              int
	selected            string
	cancelled           bool
	spinnerFrame        int
	navigationModifiers map[string]struct{}
}

const quickChromeRows = 3

func newQuickPickerModel(
	sessions []QuickSession,
	themeName string,
	navigationModifiers []string,
) quickPickerModel {
	if themeName != themeLight {
		themeName = themeDark
	}
	theme := newPickerThemeFor(themeName, colAccent)
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "❯ "
	inputStyles := input.Styles()
	inputStyles.Focused.Prompt = theme.prompt
	inputStyles.Blurred.Prompt = theme.prompt
	input.SetStyles(inputStyles)
	input.Focus()

	model := quickPickerModel{
		sessions:            sessions,
		visible:             nil,
		queryInput:          input,
		theme:               theme,
		cursor:              0,
		width:               0,
		height:              0,
		selected:            "",
		cancelled:           false,
		spinnerFrame:        0,
		navigationModifiers: modifierSet(navigationModifiers),
	}
	model = model.filtered(true)

	return model
}

func (m quickPickerModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.hasWorkingSession() {
		cmds = append(cmds, scheduleSpinner())
	}

	return tea.Batch(cmds...)
}

func (m quickPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case spinnerTickMsg:
		if m.hasWorkingSession() {
			m.spinnerFrame++

			return m, scheduleSpinner()
		}
	case tea.KeyPressMsg:
		if delta, ok := m.navigationDelta(msg); ok {
			m = m.moved(delta)

			return m, nil
		}

		switch msg.String() {
		case keyCtrlC, keyCtrlQ, keyEsc:
			m.cancelled = true

			return m, tea.Quit
		case keyEnter:
			if len(m.visible) > 0 {
				m.selected = m.visible[m.cursor].Name

				return m, tea.Quit
			}
		}
	}

	previousQuery := m.queryInput.Value()
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(msg)
	if previousQuery != m.queryInput.Value() {
		m = m.filtered(false)
	}

	return m, cmd
}

func (m quickPickerModel) View() tea.View {
	width := m.width
	if width <= 0 {
		width = 32
	}

	var buf strings.Builder
	right := strconv.Itoa(len(m.visible))
	if len(m.visible) != len(m.sessions) {
		right += "/" + strconv.Itoa(len(m.sessions))
	}
	buf.WriteString(m.theme.frameTop("sessions", right, width))
	buf.WriteString("\n")
	buf.WriteString(m.theme.frameLine(m.queryInput.View(), width))
	buf.WriteString("\n")

	if len(m.visible) == 0 {
		empty := "No saved sessions"
		if m.queryInput.Value() != "" {
			empty = "No sessions match query"
		}
		buf.WriteString(m.theme.frameLine(m.theme.faint.Render(empty), width))
		buf.WriteString("\n")
	} else {
		start, end := m.visibleRange()
		for index := start; index < end; index++ {
			buf.WriteString(m.theme.frameLine(m.renderSession(index, width), width))
			buf.WriteString("\n")
		}
	}

	buf.WriteString(m.theme.frameBottom(m.helpHints(), width))

	view := tea.NewView(buf.String())
	view.AltScreen = true

	return view
}

func (m quickPickerModel) navigationDelta(msg tea.KeyPressMsg) (int, bool) {
	switch msg.String() {
	case "down":
		return 1, true
	case "up":
		return -1, true
	case keyCtrlJ:
		_, ok := m.navigationModifiers["control"]

		return 1, ok
	case keyCtrlK:
		_, ok := m.navigationModifiers["control"]

		return -1, ok
	case "super+j":
		_, ok := m.navigationModifiers["command"]

		return 1, ok
	case "super+k":
		_, ok := m.navigationModifiers["command"]

		return -1, ok
	default:
		return 0, false
	}
}

func (m quickPickerModel) helpHints() string {
	keys := make([]string, 0, len(m.navigationModifiers))
	if _, ok := m.navigationModifiers["command"]; ok {
		keys = append(keys, "⌘j/⌘k")
	}
	if _, ok := m.navigationModifiers["control"]; ok {
		keys = append(keys, "^j/^k")
	}
	keys = append(keys, "↑/↓")

	return strings.Join(keys, " ") + " move  ·  enter switch  ·  esc quit"
}

func (m quickPickerModel) moved(delta int) quickPickerModel {
	if len(m.visible) == 0 {
		return m
	}

	m.cursor = (m.cursor + delta + len(m.visible)) % len(m.visible)

	return m
}

func (m quickPickerModel) visibleRange() (int, int) {
	available := max(1, m.height-quickChromeRows)
	if available >= len(m.visible) {
		return 0, len(m.visible)
	}

	start := m.cursor - available/2
	start = max(0, min(start, len(m.visible)-available))

	return start, start + available
}

func (m quickPickerModel) renderSession(index, width int) string {
	session := m.visible[index]
	state := "○"
	if session.Working {
		state = m.theme.statusWorking.Render(statusGlyphFrame(StatusWorking, m.spinnerFrame))
	} else if session.Restored {
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

func (m quickPickerModel) hasWorkingSession() bool {
	for _, session := range m.visible {
		if session.Working {
			return true
		}
	}

	return false
}

func (m quickPickerModel) filtered(selectCurrent bool) quickPickerModel {
	type match struct {
		session QuickSession
		score   int
	}

	query := m.queryInput.Value()
	matches := make([]match, 0, len(m.sessions))
	for _, session := range m.sessions {
		score, ok := fuzzyScore(query, session.Name)
		if !ok {
			continue
		}
		matches = append(matches, match{session: session, score: score})
	}
	if query != "" {
		sort.SliceStable(
			matches,
			func(i, j int) bool { return matches[i].score > matches[j].score },
		)
	}

	m.visible = make([]QuickSession, 0, len(matches))
	m.cursor = 0
	for index, item := range matches {
		m.visible = append(m.visible, item.session)
		if selectCurrent && item.session.Current {
			m.cursor = index
		}
	}

	return m
}

func modifierSet(modifiers []string) map[string]struct{} {
	result := make(map[string]struct{}, len(modifiers))
	for _, modifier := range modifiers {
		result[modifier] = struct{}{}
	}

	return result
}

//nolint:gochecknoglobals
var newQuickPickerRunner = func(m quickPickerModel) pickerRunner {
	return tea.NewProgram(m)
}

func ChooseQuickSession(
	sessions []QuickSession,
	themeName string,
	navigationModifiers []string,
) (string, error) {
	if tuiDisabled() {
		return "", errTUIDisabled
	}

	runner := newQuickPickerRunner(newQuickPickerModel(sessions, themeName, navigationModifiers))
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
