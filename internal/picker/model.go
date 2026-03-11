//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type pickerRow struct {
	target     Target
	item       string
	captured   string
	wins       string
	state      string
	cmd        string
	windowName string
	selectable bool
}

type pickerModel struct {
	sessions      []Session
	windowSort    []WindowSortKey
	visible       []pickerRow
	queryInput    textinput.Model
	viewport      viewport.Model
	selectedStyle lipgloss.Style
	selected      Target
	cancelled     bool
	cursor        int
	width         int
	height        int
	actions       Actions
	statusMsg     string
	mode          pickerMode
	promptInput   textinput.Model
	pending       Target
}

type pickerMode int

const (
	modeBrowse pickerMode = iota
	modeConfirmDeleteSession
	modeRenameWindow
	modeRenameSession
	modeNewSession
	modeNewWindow
)

const scrollMargin = 2

func newPickerModel(sessions []Session, windowSort []WindowSortKey, actions Actions) pickerModel {
	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "> "
	input.Focus()

	vp := viewport.New()

	m := pickerModel{
		sessions:      sessions,
		windowSort:    windowSort,
		queryInput:    input,
		viewport:      vp,
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		cursor:        0,
		actions:       actions,
		mode:          modeBrowse,
	}
	m.applyFilter()
	return m
}

func (m pickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.renderViewport()
		return m, nil
	case tea.KeyPressMsg:
		if m.mode != modeBrowse {
			return m.handlePromptKey(msg)
		}
		switch msg.String() {
		case "ctrl+c", "ctrl+q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "ctrl+d":
			if err := m.deleteCurrentWindow(); err != nil {
				m.setStatus(err.Error())
			} else {
				m.clearStatus()
			}
			m.reload()
			m.renderViewport()
			return m, nil
		case "alt+d":
			m.confirmDeleteSession()
			return m, nil
		case "ctrl+r":
			m.renameCurrentWindow()
			return m, nil
		case "alt+r":
			m.renameCurrentSession()
			return m, nil
		case "alt+n":
			m.newSession()
			return m, nil
		case "ctrl+n":
			m.newWindow()
			return m, nil
		case "ctrl+k":
			m.movePrevSelectable()
			m.ensureCursorVisible()
			m.renderViewport()
			return m, nil
		case "ctrl+j":
			m.moveNextSelectable()
			m.ensureCursorVisible()
			m.renderViewport()
			return m, nil
		case "enter":
			if len(m.visible) == 0 {
				return m, nil
			}
			if m.cursor >= 0 && m.cursor < len(m.visible) && m.visible[m.cursor].selectable {
				m.selected = m.visible[m.cursor].target
				return m, tea.Quit
			}
		}
	}

	prevQuery := m.queryInput.Value()
	var cmd tea.Cmd
	m.queryInput, cmd = m.queryInput.Update(msg)
	if prevQuery != m.queryInput.Value() {
		m.applyFilter()
		m.ensureCursorVisible()
		m.renderViewport()
	}
	return m, cmd
}

func (m pickerModel) View() tea.View {
	var b strings.Builder
	if m.mode == modeBrowse {
		b.WriteString(m.queryInput.View())
	} else {
		b.WriteString(m.promptInput.View())
	}
	b.WriteString("\n")
	layout := buildPickerTableLayout(m.tableContentWidth())
	b.WriteString("  ")
	b.WriteString(layout.header())
	b.WriteString("\n")
	if m.statusMsg != "" {
		b.WriteString(m.statusMsg)
		b.WriteString("\n")
	}
	if len(m.visible) == 0 {
		b.WriteString("No sessions or windows match query\n")
		view := tea.NewView(b.String())
		view.AltScreen = true
		return view
	}
	b.WriteString(m.viewport.View())
	view := tea.NewView(b.String())
	view.AltScreen = true
	return view
}

func (m *pickerModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.viewport.SetWidth(max(1, m.width-1))
	inputHeight := lipgloss.Height(m.queryInput.View())
	if m.mode != modeBrowse {
		inputHeight = lipgloss.Height(m.promptInput.View())
	}
	reserved := inputHeight + 1 + m.statusHeight()
	m.viewport.SetHeight(max(1, m.height-reserved))
}

func (m *pickerModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	m.visible = filteredTreeRows(m.sessions, query, m.windowSort)
	if len(m.visible) == 0 {
		m.cursor = 0
		m.viewport.SetContent("")
		return
	}
	if m.cursor < 0 || m.cursor >= len(m.visible) || !m.visible[m.cursor].selectable {
		m.cursor = nearestSelectableRow(m.visible, m.cursor)
	}
}

func (m *pickerModel) renderViewport() {
	if len(m.visible) == 0 {
		m.viewport.SetContent("")
		return
	}
	layout := buildPickerTableLayout(m.tableContentWidth())
	lines := make([]string, 0, len(m.visible))
	for i, row := range m.visible {
		pointer := "  "
		if i == m.cursor && row.selectable {
			pointer = "> "
		}
		line := pointer + layout.row(row)
		if i == m.cursor && row.selectable {
			line = m.selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *pickerModel) tableContentWidth() int {
	width := m.viewport.Width()
	if width <= 0 {
		width = m.width
	}
	if width <= 0 {
		width = 80
	}
	return max(1, width-2) // keep room for line pointer ("> ")
}

func (m *pickerModel) ensureCursorVisible() {
	if len(m.visible) == 0 || m.viewport.Height() <= 0 {
		return
	}
	maxOffset := max(0, len(m.visible)-m.viewport.Height())
	top := m.viewport.YOffset()
	bottom := top + m.viewport.Height() - 1

	if m.cursor < top+scrollMargin {
		newTop := m.cursor - scrollMargin
		if newTop < 0 {
			newTop = 0
		}
		m.viewport.SetYOffset(newTop)
		return
	}
	if m.cursor > bottom-scrollMargin {
		newTop := m.cursor - (m.viewport.Height() - 1 - scrollMargin)
		if newTop < 0 {
			newTop = 0
		}
		if newTop > maxOffset {
			newTop = maxOffset
		}
		m.viewport.SetYOffset(newTop)
	}
}

func ChooseTarget(sessions []Session, windowSort []WindowSortKey, actions Actions) (Target, error) {
	if tuiDisabled() {
		return Target{}, fmt.Errorf("TUI picker disabled in fzf-only build")
	}
	m := newPickerModel(sessions, windowSort, actions)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return Target{}, err
	}

	result, ok := finalModel.(pickerModel)
	if !ok {
		return Target{}, fmt.Errorf("unexpected picker model type")
	}
	if result.cancelled {
		return Target{}, fmt.Errorf("selection canceled")
	}
	if strings.TrimSpace(result.selected.SessionName) == "" {
		return Target{}, fmt.Errorf("no session selected")
	}
	return result.selected, nil
}
