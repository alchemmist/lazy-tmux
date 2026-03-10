package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const scrollMargin = 2

type pickerRow struct {
	target     PickerTarget
	item       string
	captured   string
	wins       string
	state      string
	cmd        string
	windowName string
	selectable bool
}

type pickerActions struct {
	DeleteWindow  func(session string, windowIndex int) error
	DeleteSession func(session string) error
	RenameWindow  func(session string, windowIndex int, name string) error
	RenameSession func(session string, name string) error
	NewSession    func(name string) error
	Reload        func() ([]pickerSession, error)
}

type pickerModel struct {
	sessions      []pickerSession
	windowSort    []WindowSortKey
	visible       []pickerRow
	queryInput    textinput.Model
	viewport      viewport.Model
	selectedStyle lipgloss.Style
	selected      PickerTarget
	cancelled     bool
	cursor        int
	width         int
	height        int
	actions       pickerActions
	statusMsg     string
	mode          pickerMode
	promptInput   textinput.Model
	pending       PickerTarget
}

type pickerMode int

const (
	modeBrowse pickerMode = iota
	modeConfirmDeleteSession
	modeRenameWindow
	modeRenameSession
	modeNewSession
)

func newPickerModel(sessions []pickerSession, windowSort []WindowSortKey, actions pickerActions) pickerModel {
	input := textinput.New()
	input.Placeholder = "fuzzy search by session/window"
	input.Prompt = "> "
	input.Focus()

	vp := viewport.New(0, 0)

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
	case tea.KeyMsg:
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
		case "ctrl+shift+d":
			m.confirmDeleteSession()
			return m, nil
		case "ctrl+r":
			m.renameCurrentWindow()
			return m, nil
		case "ctrl+shift+r":
			m.renameCurrentSession()
			return m, nil
		case "ctrl+shift+n":
			m.newSession()
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

func (m pickerModel) View() string {
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
		return b.String()
	}
	b.WriteString(m.viewport.View())
	return b.String()
}

func (m *pickerModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	m.viewport.Width = max(1, m.width-1)
	inputHeight := lipgloss.Height(m.queryInput.View())
	if m.mode != modeBrowse {
		inputHeight = lipgloss.Height(m.promptInput.View())
	}
	reserved := inputHeight + 1 + m.statusHeight()
	m.viewport.Height = max(1, m.height-reserved)
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
		m.cursor = firstSelectableRow(m.visible)
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

func (m *pickerModel) deleteCurrentWindow() error {
	row, ok := m.currentRow()
	if !ok || row.target.WindowIndex == nil {
		return fmt.Errorf("select a window row to delete")
	}
	if m.actions.DeleteWindow == nil {
		return fmt.Errorf("delete window not available")
	}
	return m.actions.DeleteWindow(row.target.SessionName, *row.target.WindowIndex)
}

func (m *pickerModel) confirmDeleteSession() {
	row, ok := m.currentRow()
	if !ok {
		m.setStatus("select a session to delete")
		return
	}
	m.pending = row.target
	m.mode = modeConfirmDeleteSession
	m.promptInput = textinput.New()
	m.promptInput.Prompt = fmt.Sprintf("Delete session %s? type y: ", row.target.SessionName)
	m.promptInput.Focus()
	m.resize()
}

func (m *pickerModel) renameCurrentWindow() {
	row, ok := m.currentRow()
	if !ok || row.target.WindowIndex == nil {
		m.setStatus("select a window row to rename")
		return
	}
	m.pending = row.target
	m.mode = modeRenameWindow
	m.promptInput = textinput.New()
	m.promptInput.Prompt = fmt.Sprintf("Rename window %s: ", row.windowName)
	m.promptInput.SetValue(row.windowName)
	m.promptInput.CursorEnd()
	m.promptInput.Focus()
	m.resize()
}

func (m *pickerModel) renameCurrentSession() {
	row, ok := m.currentRow()
	if !ok {
		m.setStatus("select a session to rename")
		return
	}
	m.pending = row.target
	m.mode = modeRenameSession
	m.promptInput = textinput.New()
	m.promptInput.Prompt = fmt.Sprintf("Rename session %s: ", row.target.SessionName)
	m.promptInput.SetValue(row.target.SessionName)
	m.promptInput.CursorEnd()
	m.promptInput.Focus()
	m.resize()
}

func (m *pickerModel) newSession() {
	m.pending = PickerTarget{}
	m.mode = modeNewSession
	m.promptInput = textinput.New()
	m.promptInput.Prompt = "New session name: "
	m.promptInput.Focus()
	m.resize()
}

func (m pickerModel) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeBrowse
		m.promptInput.Blur()
		m.resize()
		return m, nil
	case "enter":
		if m.mode == modeConfirmDeleteSession {
			val := strings.TrimSpace(m.promptInput.Value())
			if strings.EqualFold(val, "y") {
				if err := m.deleteSession(m.pending.SessionName); err != nil {
					m.setStatus(err.Error())
				} else {
					m.clearStatus()
				}
				m.reload()
				m.renderViewport()
			}
		} else if m.mode == modeRenameWindow {
			name := strings.TrimSpace(m.promptInput.Value())
			if name != "" && m.pending.WindowIndex != nil {
				if err := m.renameWindow(m.pending.SessionName, *m.pending.WindowIndex, name); err != nil {
					m.setStatus(err.Error())
				} else {
					m.clearStatus()
				}
				m.reload()
				m.renderViewport()
			}
		} else if m.mode == modeRenameSession {
			name := strings.TrimSpace(m.promptInput.Value())
			if name != "" {
				if err := m.renameSession(m.pending.SessionName, name); err != nil {
					m.setStatus(err.Error())
				} else {
					m.clearStatus()
				}
				m.reload()
				m.renderViewport()
			}
		} else if m.mode == modeNewSession {
			name := strings.TrimSpace(m.promptInput.Value())
			if name != "" {
				if err := m.createSession(name); err != nil {
					m.setStatus(err.Error())
				} else {
					m.clearStatus()
				}
				m.reload()
				m.renderViewport()
			}
		}
		m.mode = modeBrowse
		m.promptInput.Blur()
		m.resize()
		return m, nil
	}
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	return m, cmd
}

func (m *pickerModel) deleteSession(session string) error {
	if m.actions.DeleteSession == nil {
		return fmt.Errorf("delete session not available")
	}
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("select a session to delete")
	}
	return m.actions.DeleteSession(session)
}

func (m *pickerModel) renameWindow(session string, windowIndex int, name string) error {
	if m.actions.RenameWindow == nil {
		return fmt.Errorf("rename window not available")
	}
	return m.actions.RenameWindow(session, windowIndex, name)
}

func (m *pickerModel) renameSession(session string, name string) error {
	if m.actions.RenameSession == nil {
		return fmt.Errorf("rename session not available")
	}
	return m.actions.RenameSession(session, name)
}

func (m *pickerModel) createSession(name string) error {
	if m.actions.NewSession == nil {
		return fmt.Errorf("new session not available")
	}
	return m.actions.NewSession(name)
}

func (m *pickerModel) reload() {
	if m.actions.Reload == nil {
		return
	}
	sessions, err := m.actions.Reload()
	if err != nil {
		m.setStatus(err.Error())
		return
	}
	m.sessions = sessions
	m.applyFilter()
	m.ensureCursorVisible()
}

func (m *pickerModel) currentRow() (pickerRow, bool) {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return pickerRow{}, false
	}
	return m.visible[m.cursor], true
}

func (m *pickerModel) setStatus(msg string) {
	m.statusMsg = strings.TrimSpace(msg)
	m.resize()
}

func (m *pickerModel) clearStatus() {
	m.statusMsg = ""
	m.resize()
}

func (m *pickerModel) statusHeight() int {
	if m.statusMsg == "" {
		return 0
	}
	return 1
}

func (m *pickerModel) tableContentWidth() int {
	width := m.viewport.Width
	if width <= 0 {
		width = m.width
	}
	if width <= 0 {
		width = 80
	}
	return max(1, width-2) // keep room for line pointer ("> ")
}

func (m *pickerModel) ensureCursorVisible() {
	if len(m.visible) == 0 || m.viewport.Height <= 0 {
		return
	}
	maxOffset := max(0, len(m.visible)-m.viewport.Height)
	top := m.viewport.YOffset
	bottom := top + m.viewport.Height - 1

	if m.cursor < top+scrollMargin {
		newTop := m.cursor - scrollMargin
		if newTop < 0 {
			newTop = 0
		}
		m.viewport.YOffset = newTop
		return
	}
	if m.cursor > bottom-scrollMargin {
		newTop := m.cursor - (m.viewport.Height - 1 - scrollMargin)
		if newTop < 0 {
			newTop = 0
		}
		if newTop > maxOffset {
			newTop = maxOffset
		}
		m.viewport.YOffset = newTop
	}
}

func filteredTreeRows(sessions []pickerSession, query string, windowSort []WindowSortKey) []pickerRow {
	rows := make([]pickerRow, 0)
	for _, s := range sessions {
		windows := make([]snapshot.Window, len(s.Windows))
		copy(windows, s.Windows)
		sortWindows(windows, windowSort)

		sessionMatch := query == "" || fuzzyMatch(query, strings.ToLower(s.Record.SessionName))
		matchedWindows := make([]snapshot.Window, 0, len(windows))
		for _, w := range windows {
			target := strings.ToLower(s.Record.SessionName + " " + w.Name)
			if query == "" || sessionMatch || fuzzyMatch(query, target) {
				matchedWindows = append(matchedWindows, w)
			}
		}

		if !sessionMatch && len(matchedWindows) == 0 {
			continue
		}

		rows = append(rows, pickerRow{
			target:     PickerTarget{SessionName: s.Record.SessionName},
			item:       s.Record.SessionName,
			captured:   s.Record.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			wins:       fmt.Sprintf("%d", s.Record.Windows),
			state:      sessionStateIcon(s.Restored),
			selectable: false,
		})

		for i, w := range matchedWindows {
			branch := "├─"
			if i == len(matchedWindows)-1 {
				branch = "╰─"
			}
			wi := w.Index
			rows = append(rows, pickerRow{
				target:     PickerTarget{SessionName: s.Record.SessionName, WindowIndex: &wi},
				item:       fmt.Sprintf("  %s [%d] %s", branch, w.Index, w.Name),
				captured:   "",
				wins:       "",
				state:      "",
				cmd:        windowPreviewCommand(w),
				windowName: w.Name,
				selectable: true,
			})
		}
	}
	return rows
}

func windowPreviewCommand(w snapshot.Window) string {
	if len(w.Panes) == 0 {
		return ""
	}

	// Snapshot may have sparse pane indices; fall back to first pane if active is missing.
	active := 0
	for i := range w.Panes {
		if w.Panes[i].Index == w.ActivePane {
			active = i
			break
		}
	}

	if cmd := strings.TrimSpace(w.Panes[active].RestoreCmd); cmd != "" {
		return cmd
	}
	return strings.TrimSpace(w.Panes[active].CurrentCmd)
}

func sessionStateIcon(restored bool) string {
	if restored {
		return "✓"
	}
	return ""
}

func fuzzyMatch(query, target string) bool {
	if query == "" {
		return true
	}
	qi := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

func chooseTarget(sessions []pickerSession, windowSort []WindowSortKey, actions pickerActions) (PickerTarget, error) {
	m := newPickerModel(sessions, windowSort, actions)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return PickerTarget{}, err
	}

	result, ok := finalModel.(pickerModel)
	if !ok {
		return PickerTarget{}, fmt.Errorf("unexpected picker model type")
	}
	if result.cancelled {
		return PickerTarget{}, fmt.Errorf("selection canceled")
	}
	if strings.TrimSpace(result.selected.SessionName) == "" {
		return PickerTarget{}, fmt.Errorf("no session selected")
	}
	return result.selected, nil
}

func firstSelectableRow(rows []pickerRow) int {
	for i := range rows {
		if rows[i].selectable {
			return i
		}
	}
	return 0
}

func (m *pickerModel) moveNextSelectable() {
	if len(m.visible) == 0 {
		return
	}
	for i := m.cursor + 1; i < len(m.visible); i++ {
		if m.visible[i].selectable {
			m.cursor = i
			return
		}
	}
}

func (m *pickerModel) movePrevSelectable() {
	if len(m.visible) == 0 {
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.visible[i].selectable {
			m.cursor = i
			return
		}
	}
}

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[:n])
	}
	return string(r[:n-3]) + "..."
}
