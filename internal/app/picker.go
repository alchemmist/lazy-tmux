package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const scrollMargin = 2

type pickerRow struct {
	target     PickerTarget
	item       string
	captured   string
	wins       string
	selectable bool
}

type pickerModel struct {
	sessions      []pickerSession
	visible       []pickerRow
	queryInput    textinput.Model
	viewport      viewport.Model
	selectedStyle lipgloss.Style
	selected      PickerTarget
	cancelled     bool
	cursor        int
	width         int
	height        int
}

func newPickerModel(sessions []pickerSession) pickerModel {
	input := textinput.New()
	input.Placeholder = "fuzzy search by session/window"
	input.Prompt = "> "
	input.Focus()

	vp := viewport.New(0, 0)

	m := pickerModel{
		sessions:      sessions,
		queryInput:    input,
		viewport:      vp,
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		cursor:        0,
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
		switch msg.String() {
		case "ctrl+c", "ctrl+q", "esc":
			m.cancelled = true
			return m, tea.Quit
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
	b.WriteString(m.queryInput.View())
	b.WriteString("\n")
	itemW := m.itemWidth()
	b.WriteString("  ")
	b.WriteString(fmt.Sprintf("%-*s %-19s %4s\n", itemW, "ITEM", "CAPTURED", "WINS"))
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
	reserved := lipgloss.Height(m.queryInput.View()) + 1
	m.viewport.Height = max(1, m.height-reserved)
}

func (m *pickerModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	m.visible = filteredTreeRows(m.sessions, query)
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
	itemW := m.itemWidth()
	lines := make([]string, 0, len(m.visible))
	for i, row := range m.visible {
		pointer := "  "
		if i == m.cursor && row.selectable {
			pointer = "> "
		}
		line := pointer + fmt.Sprintf("%-*s %-19s %4s", itemW, trim(row.item, itemW), row.captured, row.wins)
		if i == m.cursor && row.selectable {
			line = m.selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *pickerModel) itemWidth() int {
	return max(16, m.viewport.Width-28)
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

func filteredTreeRows(sessions []pickerSession, query string) []pickerRow {
	rows := make([]pickerRow, 0)
	for _, s := range sessions {
		windows := make([]snapshot.Window, len(s.Windows))
		copy(windows, s.Windows)
		sort.Slice(windows, func(i, j int) bool { return windows[i].Index < windows[j].Index })

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
				selectable: true,
			})
		}
	}
	return rows
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

func chooseTarget(sessions []pickerSession) (PickerTarget, error) {
	m := newPickerModel(sessions)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
