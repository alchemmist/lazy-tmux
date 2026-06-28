//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
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
	sessions    []Session
	windowSort  []WindowSortKey
	visible     []pickerRow
	queryInput  textinput.Model
	viewport    viewport.Model
	theme       pickerTheme
	selected    Target
	cancelled   bool
	cursor      int
	width       int
	height      int
	actions     Actions
	statusMsg   string
	mode        pickerMode
	promptInput textinput.Model
	pending     Target
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
	theme := newPickerTheme()

	input := textinput.New()
	input.Placeholder = ""
	input.Prompt = "❯ "

	inputStyles := input.Styles()
	inputStyles.Focused.Prompt = theme.prompt
	inputStyles.Blurred.Prompt = theme.prompt
	input.SetStyles(inputStyles)
	input.Focus()

	viewPort := viewport.New()

	model := pickerModel{
		sessions:   sessions,
		windowSort: windowSort,
		queryInput: input,
		viewport:   viewPort,
		theme:      theme,
		cursor:     0,
		actions:    actions,
		mode:       modeBrowse,
	}
	model.applyFilter()

	return model
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
	case tea.MouseWheelMsg:
		if m.mode != modeBrowse {
			return m, nil
		}

		switch msg.Button {
		case tea.MouseWheelUp:
			m.movePrevSelectable()
		case tea.MouseWheelDown:
			m.moveNextSelectable()
		}

		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil
	case tea.MouseClickMsg:
		if m.mode != modeBrowse || msg.Button != tea.MouseLeft {
			return m, nil
		}

		idx, ok := m.rowAtY(msg.Y)
		if !ok {
			return m, nil
		}

		if idx == m.cursor {
			m.selected = m.visible[idx].target
			return m, tea.Quit
		}

		m.cursor = idx
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil
	case tea.KeyPressMsg:
		if m.mode != modeBrowse {
			return m.handlePromptKey(msg)
		}

		if next, cmd, handled := m.handleBrowseKey(msg); handled {
			return next, cmd
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
	width := m.width
	if width <= 0 {
		width = 80
	}

	var buf strings.Builder

	// Top border: title + session/window counts.
	sessions, windows := m.counts()
	right := m.theme.count.Render(fmt.Sprintf("%d sessions · %d windows", sessions, windows))
	buf.WriteString(m.theme.frameTop("lazy-tmux", right, width))
	buf.WriteString("\n")

	// Search input (or the active action prompt).
	input := m.queryInput.View()
	if m.mode != modeBrowse {
		input = m.promptInput.View()
	}

	buf.WriteString(m.theme.frameLine(input, width))
	buf.WriteString("\n")

	// Column header, aligned with the row name column (2 leading cells).
	layout := buildPickerTableLayout(m.tableContentWidth())
	buf.WriteString(m.theme.frameLine("  "+layout.styledHeader(m.theme), width))
	buf.WriteString("\n")

	if m.statusMsg != "" {
		buf.WriteString(m.theme.frameLine(m.theme.statusErr.Render(m.statusMsg), width))
		buf.WriteString("\n")
	}

	if len(m.visible) == 0 {
		buf.WriteString(
			m.theme.frameLine(m.theme.faint.Render("No sessions or windows match query"), width),
		)
		buf.WriteString("\n")
	} else {
		for line := range strings.SplitSeq(m.viewport.View(), "\n") {
			buf.WriteString(m.theme.frameLine(line, width))
			buf.WriteString("\n")
		}
	}

	// Bottom border: key hints.
	buf.WriteString(m.theme.frameBottom(m.helpHints(), width))

	view := tea.NewView(buf.String())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

func (m pickerModel) counts() (sessions, windows int) {
	for i := range m.sessions {
		windows += len(m.sessions[i].Windows)
	}

	return len(m.sessions), windows
}

func (m pickerModel) helpHints() string {
	pairs := []struct{ key, label string }{
		{"↵", "select"},
		{"^j/^k", "move"},
		{"^d", "del"},
		{"^r", "rename"},
		{"^n", "new"},
		{"esc", "quit"},
	}

	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, m.theme.helpKey.Render(p.key)+" "+m.theme.helpText.Render(p.label))
	}

	return strings.Join(parts, m.theme.helpText.Render("  ·  "))
}

// handleBrowseKey dispatches a key press while browsing. The bool reports
// whether the key was consumed; unconsumed keys fall through to the search
// input so typing still filters the list.
func (m pickerModel) handleBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q", "esc":
		m.cancelled = true
		return m, tea.Quit, true
	case "ctrl+d":
		m.applyActionResult(m.deleteCurrentWindow())
		return m, nil, true
	case "alt+d":
		m.confirmDeleteSession()
		return m, nil, true
	case "ctrl+r":
		m.renameCurrentWindow()
		return m, nil, true
	case "alt+r":
		m.renameCurrentSession()
		return m, nil, true
	case "alt+n":
		m.newSession()
		return m, nil, true
	case "ctrl+n":
		m.newWindow()
		return m, nil, true
	case "alt+w":
		m.applyActionResult(m.wakeupSession())
		return m, nil, true
	case "alt+s":
		m.applyActionResult(m.sleepSession())
		return m, nil, true
	case "ctrl+k":
		m.movePrevSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case "ctrl+j":
		m.moveNextSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.visible) && m.visible[m.cursor].selectable {
			m.selected = m.visible[m.cursor].target
			return m, tea.Quit, true
		}
	}

	return m, nil, false
}

// applyActionResult surfaces an action error as a status message (or clears it),
// then reloads sessions and re-renders.
func (m *pickerModel) applyActionResult(err error) {
	if err != nil {
		m.setStatus(err.Error())
	} else {
		m.clearStatus()
	}

	m.reload()
	m.renderViewport()
}

func (m *pickerModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	m.viewport.SetWidth(m.contentWidth())

	// Reserved chrome rows: top border, input, header, bottom border (+ status).
	reserved := 4 + m.statusHeight()
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
	barWidth := max(0, m.contentWidth()-2) // room for the stripe + a space
	lines := make([]string, 0, len(m.visible))

	for rowIndex, row := range m.visible {
		if rowIndex == m.cursor && row.selectable {
			body := layout.row(row)

			if pad := barWidth - len([]rune(body)); pad > 0 {
				body += strings.Repeat(" ", pad)
			}

			lines = append(lines, m.theme.stripe.Render("▌")+m.theme.selBar.Render(" "+body))

			continue
		}

		lines = append(lines, "  "+layout.styledRow(row, m.theme))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// contentWidth is the usable width inside the frame's side borders and gutter
// spaces (│ … │), i.e. the width each inner line is padded to.
func (m *pickerModel) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}

	return max(1, width-4) // 2 borders + 2 gutter spaces
}

// tableContentWidth is the width available to the column table, after the 2
// leading cells reserved for the selection stripe / row indent.
func (m *pickerModel) tableContentWidth() int {
	return max(1, m.contentWidth()-2)
}

// rowAtY maps a mouse Y coordinate to a selectable row index. The list starts
// below the top border, input and header (and the optional status line).
func (m *pickerModel) rowAtY(mouseY int) (int, bool) {
	rowStart := 3 + m.statusHeight() // top border + input + header [+ status]

	if mouseY < rowStart {
		return 0, false
	}

	idx := m.viewport.YOffset() + (mouseY - rowStart)
	if idx < 0 || idx >= len(m.visible) || !m.visible[idx].selectable {
		return 0, false
	}

	return idx, true
}

func (m *pickerModel) ensureCursorVisible() {
	if len(m.visible) == 0 || m.viewport.Height() <= 0 {
		return
	}

	maxOffset := max(0, len(m.visible)-m.viewport.Height())
	top := m.viewport.YOffset()
	bottom := top + m.viewport.Height() - 1

	if m.cursor < top+scrollMargin {
		newTop := max(m.cursor-scrollMargin, 0)

		m.viewport.SetYOffset(newTop)

		return
	}

	if m.cursor > bottom-scrollMargin {
		newTop := min(max(m.cursor-(m.viewport.Height()-1-scrollMargin), 0), maxOffset)

		m.viewport.SetYOffset(newTop)
	}
}

type pickerRunner interface {
	Run() (tea.Model, error)
}

var newPickerRunner = func(m pickerModel) pickerRunner {
	return tea.NewProgram(m)
}

func ChooseTarget(sessions []Session, windowSort []WindowSortKey, actions Actions) (Target, error) {
	if tuiDisabled() {
		return Target{}, fmt.Errorf("TUI picker disabled in fzf-only build")
	}

	m := newPickerModel(sessions, windowSort, actions)
	runner := newPickerRunner(m)

	finalModel, err := runner.Run()
	if err != nil {
		return Target{}, fmt.Errorf("run picker: %w", err)
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
