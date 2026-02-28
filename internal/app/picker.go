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
	record snapshot.Record
	score  int
}

type pickerModel struct {
	allRows    []pickerRow
	visible    []pickerRow
	queryInput textinput.Model
	table      table.Model
	selected   string
	cancelled  bool
	width      int
	height     int
}

func newPickerModel(records []snapshot.Record) pickerModel {
	input := textinput.New()
	input.Placeholder = "fuzzy search"
	input.Prompt = "query> "
	input.Focus()

	cols := []table.Column{
		{Title: "SESSION", Width: 32},
		{Title: "CAPTURED", Width: 19},
		{Title: "WINS", Width: 6},
		{Title: "PANES", Width: 6},
	}

	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(16),
	)

	m := pickerModel{
		queryInput: input,
		table:      tbl,
		allRows:    make([]pickerRow, 0, len(records)),
	}

	for _, r := range records {
		m.allRows = append(m.allRows, pickerRow{record: r, score: 0})
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
		case "enter":
			if len(m.visible) == 0 {
				return m, nil
			}
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.visible) {
				m.selected = m.visible[idx].record.SessionName
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

	var cmdTable tea.Cmd
	m.table, cmdTable = m.table.Update(msg)

	return m, tea.Batch(cmdInput, cmdTable)
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString("lazy-tmux picker\n")
	b.WriteString("enter: restore  esc/q/ctrl-c: cancel  up/down or j/k: move\n\n")
	b.WriteString(m.queryInput.View())
	b.WriteString("\n\n")
	if len(m.visible) == 0 {
		b.WriteString("No sessions match query\n")
		return b.String()
	}
	b.WriteString(m.table.View())
	return b.String()
}

func (m *pickerModel) resize() {
	if m.width <= 0 {
		return
	}
	sessionW := m.width - 45
	if sessionW < 16 {
		sessionW = 16
	}
	cols := m.table.Columns()
	if len(cols) == 4 {
		cols[0].Width = sessionW
		m.table.SetColumns(cols)
	}

	tableHeight := m.height - 7
	if tableHeight < 5 {
		tableHeight = 5
	}
	m.table.SetHeight(tableHeight)
}

func (m *pickerModel) applyFilter() {
	query := strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	rows := make([]pickerRow, 0, len(m.allRows))

	for _, row := range m.allRows {
		target := strings.ToLower(fmt.Sprintf("%s %s %dw %dp", row.record.SessionName, row.record.CapturedAt.Local().Format("2006-01-02 15:04:05"), row.record.Windows, row.record.Panes))
		score, ok := fuzzyScore(query, target)
		if !ok {
			continue
		}
		row.score = score
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].score == rows[j].score {
			return rows[i].record.CapturedAt.After(rows[j].record.CapturedAt)
		}
		return rows[i].score > rows[j].score
	})

	m.visible = rows
	tableRows := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, table.Row{
			trim(row.record.SessionName, 80),
			row.record.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%d", row.record.Windows),
			fmt.Sprintf("%d", row.record.Panes),
		})
	}
	m.table.SetRows(tableRows)

	if len(tableRows) == 0 {
		m.table.SetCursor(0)
		return
	}
	if m.table.Cursor() >= len(tableRows) {
		m.table.SetCursor(len(tableRows) - 1)
	}
}

func fuzzyScore(query, target string) (int, bool) {
	if query == "" {
		return 1, true
	}
	qi := 0
	score := 0
	streak := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			score += 10 + streak*3
			streak++
			qi++
		} else {
			streak = 0
		}
	}
	if qi != len(query) {
		return 0, false
	}
	return score, true
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

func chooseSession(records []snapshot.Record) (string, error) {
	m := newPickerModel(records)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	result, ok := finalModel.(pickerModel)
	if !ok {
		return "", fmt.Errorf("unexpected picker model type")
	}
	if result.cancelled {
		return "", fmt.Errorf("selection canceled")
	}
	if strings.TrimSpace(result.selected) == "" {
		return "", fmt.Errorf("no session selected")
	}
	return result.selected, nil
}
