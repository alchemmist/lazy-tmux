package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type pickerRow struct {
	target     PickerTarget
	item       string
	captured   string
	wins       string
	selectable bool
}

type pickerModel struct {
	sessions   []pickerSession
	visible    []pickerRow
	queryInput textinput.Model
	table      table.Model
	selected   PickerTarget
	cancelled  bool
	width      int
	height     int
}

func newPickerModel(sessions []pickerSession) pickerModel {
	input := textinput.New()
	input.Placeholder = "fuzzy search by session/window"
	input.Prompt = "> "
	input.Focus()

	cols := []table.Column{
		{Title: "ITEM", Width: 42},
		{Title: "CAPTURED", Width: 19},
		{Title: "WINS", Width: 6},
	}

	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(16),
	)

	m := pickerModel{
		sessions:   sessions,
		queryInput: input,
		table:      tbl,
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
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.cancelled = true
			return m, tea.Quit
		case "ctrl+k":
			m.movePrevSelectable()
			return m, nil
		case "ctrl+j":
			m.moveNextSelectable()
			return m, nil
		case "enter":
			if len(m.visible) == 0 {
				return m, nil
			}
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visible) && m.visible[idx].selectable {
				m.selected = m.visible[idx].target
				return m, tea.Quit
			}
		}
	}

	prevQuery := m.queryInput.Value()
	var cmdInput tea.Cmd
	m.queryInput, cmdInput = m.queryInput.Update(msg)
	if prevQuery != m.queryInput.Value() {
		m.applyFilter()
	}
	return m, cmdInput
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString(m.queryInput.View())
	b.WriteString("\n")
	if len(m.visible) == 0 {
		b.WriteString("No sessions or windows match query\n")
		return b.String()
	}
	b.WriteString(m.table.View())
	return b.String()
}

func (m *pickerModel) resize() {
	if m.width <= 0 {
		return
	}
	itemW := m.width - 34
	if itemW < 16 {
		itemW = 16
	}
	cols := m.table.Columns()
	if len(cols) == 3 {
		cols[0].Width = itemW
		m.table.SetColumns(cols)
	}

	tableHeight := m.height - 1
	if tableHeight < 5 {
		tableHeight = 5
	}
	m.table.SetHeight(tableHeight)
}

func (m *pickerModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	rows := filteredTreeRows(m.sessions, query)
	m.visible = rows

	tableRows := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, table.Row{row.item, row.captured, row.wins})
	}
	m.table.SetRows(tableRows)

	if len(tableRows) == 0 {
		m.table.SetCursor(0)
		return
	}
	cur := m.table.Cursor()
	if cur < 0 || cur >= len(tableRows) || !rows[cur].selectable {
		m.table.SetCursor(firstSelectableRow(rows))
		return
	}
	m.table.SetCursor(cur)
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
	cur := m.table.Cursor()
	for i := cur + 1; i < len(m.visible); i++ {
		if m.visible[i].selectable {
			m.table.SetCursor(i)
			return
		}
	}
}

func (m *pickerModel) movePrevSelectable() {
	if len(m.visible) == 0 {
		return
	}
	cur := m.table.Cursor()
	for i := cur - 1; i >= 0; i-- {
		if m.visible[i].selectable {
			m.table.SetCursor(i)
			return
		}
	}
}
