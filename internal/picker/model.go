//go:build !lazy_fzf

package picker

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

var (
	errTUIDisabled       = errors.New("TUI picker disabled in fzf-only build")
	errUnexpectedModel   = errors.New("unexpected picker model type")
	errSelectionCanceled = errors.New("selection canceled")
	errNoSessionSelected = errors.New("no session selected")
)

const statusRefreshInterval = 2 * time.Second

const spinnerInterval = 160 * time.Millisecond

type statusTickMsg struct{}

type spinnerTickMsg struct{}

type pickerRow struct {
	target     Target
	item       string
	captured   string
	wins       string
	state      string
	cmd        string
	windowName string
	status     WindowStatus
	selectable bool
	synthetic  bool
}

//nolint:recvcheck // deliberate value/pointer receiver mix, see above
type pickerModel struct {
	sessions     []Session
	windowSort   []WindowSortKey
	visible      []pickerRow
	queryInput   textinput.Model
	viewport     viewport.Model
	theme        pickerTheme
	themeName    string
	selected     Target
	cancelled    bool
	cursor       int
	width        int
	height       int
	actions      Actions
	statusMsg    string
	mode         pickerMode
	action       actionMode
	marked       map[string]struct{}
	palette      bool
	paletteIdx   int
	promptInput  textinput.Model
	pending      Target
	helpOpen     bool
	spinnerFrame int
	spinnerOn    bool
}

type pickerMode int

const (
	modeBrowse pickerMode = iota
	modeRenameWindow
	modeRenameSession
	modeNewSession
	modeNewWindow
)

const scrollMargin = 2

const (
	keyEnter = "enter"
	keyEsc   = "esc"
	keyCtrlC = "ctrl+c"
	keyCtrlJ = "ctrl+j"
	keyCtrlK = "ctrl+k"
)

const hintMoveKeys = "^j/^k"

const chromeRowsAboveList = 3

func newPickerModel(sessions []Session, windowSort []WindowSortKey, actions Actions) pickerModel {
	return newPickerModelWithTheme(sessions, windowSort, actions, themeDark)
}

func newPickerModelWithTheme(
	sessions []Session,
	windowSort []WindowSortKey,
	actions Actions,
	themeName string,
) pickerModel {
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

	viewPort := viewport.New()

	model := pickerModel{
		sessions:   sessions,
		windowSort: windowSort,
		queryInput: input,
		viewport:   viewPort,
		theme:      theme,
		themeName:  themeName,
		cursor:     0,
		actions:    actions,
		mode:       modeBrowse,
		action:     actionBrowse,
		marked:     make(map[string]struct{}),
		spinnerOn:  true,
	}
	model.applyFilter()

	return model
}

func (m pickerModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, scheduleStatusRefresh(), scheduleSpinner())
}

func scheduleStatusRefresh() tea.Cmd {
	return tea.Tick(statusRefreshInterval, func(time.Time) tea.Msg { return statusTickMsg{} })
}

func scheduleSpinner() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.renderViewport()

		return m, nil
	case statusTickMsg:
		return m.handleStatusTick()
	case spinnerTickMsg:
		return m.handleSpinnerTick()
	case tea.MouseWheelMsg:
		return m.handleWheel(msg)
	case tea.MouseClickMsg:
		return m.handleClick(msg)
	case tea.KeyPressMsg:
		if m.mode != modeBrowse {
			return m.handlePromptKey(msg)
		}

		if m.helpOpen {
			m.helpOpen = false
			m.renderViewport()

			return m, nil
		}

		if next, cmd, handled := m.dispatchBrowseKey(msg); handled {
			return next, cmd
		}
	}

	prevQuery := m.queryInput.Value()

	var cmd tea.Cmd

	m.queryInput, cmd = m.queryInput.Update(msg)
	if prevQuery != m.queryInput.Value() {
		m.syncPalette()
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

	sessions, windows := m.counts()
	right := m.theme.count.Render(fmt.Sprintf("%d sessions · %d windows", sessions, windows))
	buf.WriteString(m.theme.frameTop(m.titleText(), right, width))
	buf.WriteString("\n")

	input := m.queryInput.View()
	if m.mode != modeBrowse {
		input = m.promptInput.View()
	}

	buf.WriteString(m.theme.frameLine(input, width))
	buf.WriteString("\n")

	switch {
	case m.helpOpen:
		m.writeHelp(&buf, width)
	case m.palette:
		m.writePalette(&buf, width)
	default:
		m.writeTable(&buf, width)
	}

	buf.WriteString(m.theme.frameBottom(m.helpHints(), width))

	view := tea.NewView(buf.String())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

func (m pickerModel) handleStatusTick() (tea.Model, tea.Cmd) {
	if m.mode == modeBrowse && !m.palette {
		m.reload()
		m.renderViewport()
	}

	cmds := []tea.Cmd{scheduleStatusRefresh()}
	if m.hasWorkingWindow() && !m.spinnerOn {
		m.spinnerOn = true
		cmds = append(cmds, scheduleSpinner())
	}

	return m, tea.Batch(cmds...)
}

func (m pickerModel) handleSpinnerTick() (tea.Model, tea.Cmd) {
	if !m.hasWorkingWindow() {
		m.spinnerOn = false

		return m, nil
	}

	m.spinnerFrame++

	if m.mode == modeBrowse && !m.palette {
		m.applyFilter()
		m.renderViewport()
	}

	return m, scheduleSpinner()
}

func (m pickerModel) hasWorkingWindow() bool {
	for _, sess := range m.sessions {
		for _, status := range sess.Statuses {
			if status == StatusWorking {
				return true
			}
		}
	}

	return false
}

func (m pickerModel) handleWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeBrowse || m.palette || m.helpOpen {
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
}

func (m pickerModel) handleClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeBrowse || m.palette || m.helpOpen || msg.Button != tea.MouseLeft {
		return m, nil
	}

	idx, ok := m.rowAtY(msg.Y)
	if !ok {
		return m, nil
	}

	return m.handleRowClick(idx)
}

func (m pickerModel) dispatchBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.palette {
		next, handled := m.handlePaletteKey(msg)

		return next, nil, handled
	}

	if m.action != actionBrowse {
		return m.handleActionKey(msg)
	}

	return m.handleBrowseKey(msg)
}

func (m pickerModel) titleText() string {
	if cmd, ok := commandForMode(m.action); ok {
		return "lazy-tmux · " + cmd.label
	}

	return "lazy-tmux"
}

func (m pickerModel) writeTable(buf *strings.Builder, width int) {
	layout := buildPickerTableLayout(m.tableContentWidth())
	lead := "  " + strings.Repeat(" ", m.markerWidth())
	buf.WriteString(m.theme.frameLine(lead+layout.styledHeader(m.theme), width))
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

		return
	}

	for line := range strings.SplitSeq(m.viewport.View(), "\n") {
		buf.WriteString(m.theme.frameLine(line, width))
		buf.WriteString("\n")
	}
}

func (m pickerModel) writePalette(buf *strings.Builder, width int) {
	matches := matchCommands(m.commandPrefix())
	if len(matches) == 0 {
		buf.WriteString(m.theme.frameLine(m.theme.faint.Render("no matching command"), width))
		buf.WriteString("\n")

		return
	}

	for idx, cmd := range matches {
		name := m.theme.title.Render("/" + cmd.name)
		line := name + "  " + m.theme.helpKey.Render(
			cmd.chord,
		) + "  " + m.theme.meta.Render(
			cmd.desc,
		)

		if idx == m.paletteIdx {
			line = m.theme.stripe.Render("▌") + " " + line
		} else {
			line = "  " + line
		}

		buf.WriteString(m.theme.frameLine(line, width))
		buf.WriteString("\n")
	}
}

type helpEntry struct{ keys, label string }

type helpSection struct {
	title   string
	entries []helpEntry
}

//nolint:gochecknoglobals // static help-panel table, never mutated
var helpSections = []helpSection{
	{
		title: "Navigate",
		entries: []helpEntry{
			{"^j / ^k", "move down / up"},
			{"↵", "restore the selected session/window"},
			{"type", "filter the list"},
			{"esc", "quit"},
		},
	},
	{
		title: "Act on the row under the cursor",
		entries: []helpEntry{
			{"^d / ⌥d", "delete window / session"},
			{"^r / ⌥r", "rename window / session"},
			{"^n / ⌥n", "new window / session"},
			{"⌥w", "wake a sleeping session"},
			{"⌥s", "sleep a live session"},
			{"/", "command palette (same actions, typed)"},
		},
	},
}

func (m pickerModel) writeHelp(buf *strings.Builder, width int) {
	keyW := 0
	for _, section := range helpSections {
		for _, entry := range section.entries {
			if w := displayWidth(entry.keys); w > keyW {
				keyW = w
			}
		}
	}

	for sectionIdx, section := range helpSections {
		if sectionIdx > 0 {
			buf.WriteString(m.theme.frameLine("", width))
			buf.WriteString("\n")
		}

		buf.WriteString(m.theme.frameLine(m.theme.title.Render(section.title), width))
		buf.WriteString("\n")

		for _, entry := range section.entries {
			keyCell := entry.keys + strings.Repeat(" ", keyW-displayWidth(entry.keys))
			line := "  " + m.theme.helpKey.Render(
				keyCell,
			) + "   " + m.theme.meta.Render(
				entry.label,
			)
			buf.WriteString(m.theme.frameLine(line, width))
			buf.WriteString("\n")
		}
	}
}

func (m pickerModel) handleRowClick(idx int) (tea.Model, tea.Cmd) {
	if m.action == actionBrowse {
		if idx == m.cursor {
			m.selected = m.visible[idx].target

			return m, tea.Quit
		}

		m.cursor = idx
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil
	}

	m.cursor = idx

	if m.action == actionDelete {
		m.toggleMark(m.visible[idx])
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil
	}

	return m.commitAction()
}

func (m *pickerModel) syncPalette() {
	m.palette = m.action == actionBrowse && strings.HasPrefix(m.queryInput.Value(), "/")
	if !m.palette {
		m.paletteIdx = 0

		return
	}

	if matches := matchCommands(m.commandPrefix()); m.paletteIdx >= len(matches) {
		m.paletteIdx = max(0, len(matches)-1)
	}
}

func (m pickerModel) commandPrefix() string {
	return strings.TrimPrefix(m.queryInput.Value(), "/")
}

func (m pickerModel) counts() (int, int) {
	windows := 0
	for i := range m.sessions {
		windows += len(m.sessions[i].Windows)
	}

	return len(m.sessions), windows
}

func (m pickerModel) helpHints() string {
	hintMove := hint{hintMoveKeys, "move"}
	hintCancel := hint{keyEsc, "cancel"}

	var pairs []hint

	switch {
	case m.palette:
		pairs = []hint{{"↵", "run"}, {"tab", "complete"}, hintMove, hintCancel}
	case m.action != actionBrowse:
		cmd, _ := commandForMode(m.action)
		pairs = append([]hint{{cmd.label, ""}}, cmd.hints...)
		pairs = append(pairs, hintMove, hintCancel)
	case m.helpOpen:
		pairs = []hint{{"any key", "close help"}}
	default:
		pairs = []hint{
			{
				"↵",
				"select",
			},
			hintMove,
			{"⌘/ctrl+digit", "jump"},
			{"/", "commands"},
			{"?", "help"},
			{keyEsc, "quit"},
		}
	}

	parts := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		if pair.label == "" {
			parts = append(parts, m.theme.helpKey.Render(pair.key))

			continue
		}

		parts = append(
			parts,
			m.theme.helpKey.Render(pair.key)+" "+m.theme.helpText.Render(pair.label),
		)
	}

	return strings.Join(parts, m.theme.helpText.Render("  ·  "))
}

func (m pickerModel) handleBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if windowIndex, ok := windowJumpIndex(msg); ok {
		if m.jumpToWindow(windowIndex) {
			m.renderViewport()
		}

		return m, nil, true
	}

	switch msg.String() {
	case keyCtrlC, "ctrl+q", keyEsc:
		m.cancelled = true

		return m, tea.Quit, true
	case "?":
		if m.queryInput.Value() == "" {
			m.helpOpen = true

			return m, nil, true
		}
	case "ctrl+d", "alt+d", "ctrl+r", "alt+r", "ctrl+n", "alt+n", "alt+w", "alt+s":
		m.enterModeShortcut(msg.String())
		m.renderViewport()

		return m, nil, true
	case keyCtrlK:
		m.movePrevSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case keyCtrlJ:
		m.moveNextSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case keyEnter:
		if m.cursor >= 0 && m.cursor < len(m.visible) && m.visible[m.cursor].selectable {
			m.selected = m.visible[m.cursor].target

			return m, tea.Quit, true
		}
	}

	return m, nil, false
}

func windowJumpIndex(msg tea.KeyPressMsg) (int, bool) {
	if msg.Mod&(tea.ModCtrl|tea.ModMeta|tea.ModSuper) == 0 {
		return 0, false
	}

	switch {
	case msg.Code >= '0' && msg.Code <= '9':
		return int(msg.Code - '0'), true
	case msg.Code >= tea.KeyKp0 && msg.Code <= tea.KeyKp9:
		return int(msg.Code - tea.KeyKp0), true
	default:
		return 0, false
	}
}

func (m *pickerModel) jumpToWindow(index int) bool {
	for rowIndex, row := range m.visible {
		if row.target.WindowIndex == nil || *row.target.WindowIndex != index {
			continue
		}

		m.cursor = rowIndex
		m.ensureCursorVisible()

		return true
	}

	return false
}

func (m *pickerModel) applyActionResult(err error) {
	if err != nil {
		m.setStatus(err.Error())
	} else {
		m.clearStatus()
	}

	m.reload()
	m.renderViewport()
}

func (m *pickerModel) enterMode(mode actionMode) {
	m.action = mode
	m.palette = false
	m.paletteIdx = 0
	m.marked = make(map[string]struct{})
	m.queryInput.SetValue("")
	m.theme = newPickerThemeFor(m.themeName, accentForMode(mode))
	m.resize()
	m.applyFilter()
	m.ensureCursorVisible()
}

func (m *pickerModel) enterModeShortcut(key string) {
	cur, _ := m.currentRow()

	switch key {
	case "ctrl+d":
		m.enterMode(actionDelete)
		m.focusWindow(cur.target)
		m.markWindow(cur.target)
	case "alt+d":
		m.enterMode(actionDelete)
		m.focusSession(cur.target.SessionName)
		m.markSession(cur.target.SessionName)
	case "ctrl+r":
		m.enterMode(actionRename)
		m.focusWindow(cur.target)
	case "alt+r":
		m.enterMode(actionRename)
		m.focusSession(cur.target.SessionName)
	case "ctrl+n":
		m.enterMode(actionNew)
		m.focusSession(cur.target.SessionName)
	case "alt+n":
		m.enterMode(actionNew)
		m.focusSynthetic()
	case "alt+w":
		m.enterMode(actionWake)
		m.focusSession(cur.target.SessionName)
	case "alt+s":
		m.enterMode(actionSleep)
		m.focusSession(cur.target.SessionName)
	}
}

func (m *pickerModel) focusWindow(target Target) {
	if target.WindowIndex == nil {
		return
	}

	for i, row := range m.visible {
		if row.target.SessionName == target.SessionName && row.target.WindowIndex != nil &&
			*row.target.WindowIndex == *target.WindowIndex {
			m.cursor = i
			m.ensureCursorVisible()

			return
		}
	}
}

func (m *pickerModel) focusSession(name string) {
	for i, row := range m.visible {
		if !row.synthetic && row.target.WindowIndex == nil && row.target.SessionName == name {
			m.cursor = i
			m.ensureCursorVisible()

			return
		}
	}
}

func (m *pickerModel) focusSynthetic() {
	for i, row := range m.visible {
		if row.synthetic {
			m.cursor = i
			m.ensureCursorVisible()

			return
		}
	}
}

func (m *pickerModel) exitMode() {
	prev, hadRow := m.currentRow()

	m.action = actionBrowse
	m.palette = false
	m.paletteIdx = 0
	m.marked = make(map[string]struct{})
	m.queryInput.SetValue("")
	m.theme = newPickerThemeFor(m.themeName, colAccent)
	m.cursor = 0
	m.resize()
	m.applyFilter()

	if hadRow && !prev.synthetic {
		if prev.target.WindowIndex != nil {
			m.focusWindow(prev.target)
		} else {
			m.focusSession(prev.target.SessionName)
			m.cursor = m.nearestSelectable(m.cursor)
		}
	}

	m.ensureCursorVisible()
	m.renderViewport()
}

func (m pickerModel) handlePaletteKey(msg tea.KeyPressMsg) (tea.Model, bool) {
	matches := matchCommands(m.commandPrefix())

	switch msg.String() {
	case keyEsc, keyCtrlC:
		m.exitMode()

		return m, true
	case keyCtrlK:
		if m.paletteIdx > 0 {
			m.paletteIdx--
		}

		return m, true
	case keyCtrlJ:
		if m.paletteIdx < len(matches)-1 {
			m.paletteIdx++
		}

		return m, true
	case "tab":
		if len(matches) > 0 {
			m.queryInput.SetValue("/" + matches[m.paletteIdx].name)
			m.queryInput.CursorEnd()
			m.syncPalette()
		}

		return m, true
	case keyEnter:
		if len(matches) > 0 {
			if matches[m.paletteIdx].name == "theme" {
				m.applyThemeCommand(
					strings.TrimSpace(strings.TrimPrefix(m.commandPrefix(), "theme")),
				)

				return m, true
			}
			m.enterMode(matches[m.paletteIdx].mode)
			m.renderViewport()
		}

		return m, true
	}

	return m, false
}

func (m *pickerModel) applyThemeCommand(arg string) {
	theme := strings.ToLower(strings.TrimSpace(arg))
	if theme != themeDark && theme != themeLight {
		m.setStatus("usage: /theme dark|light")

		return
	}
	if m.actions.SetTheme != nil {
		err := m.actions.SetTheme(theme)
		if err != nil {
			m.setStatus(err.Error())

			return
		}
	}
	m.themeName = theme
	m.theme = newPickerThemeFor(theme, colAccent)
	m.queryInput.SetValue("")
	m.syncPalette()
	m.renderViewport()
}

func (m pickerModel) handleActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case keyCtrlC, "ctrl+q":
		m.cancelled = true

		return m, tea.Quit, true
	case keyEsc:
		m.exitMode()

		return m, nil, true
	case keyCtrlK:
		m.movePrevSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case keyCtrlJ:
		m.moveNextSelectable()
		m.ensureCursorVisible()
		m.renderViewport()

		return m, nil, true
	case "space":
		if m.action != actionDelete {
			return m, nil, false
		}

		if row, ok := m.currentRow(); ok {
			m.toggleMark(row)
			m.renderViewport()
		}

		return m, nil, true
	case keyEnter:
		next, cmd := m.commitAction()

		return next, cmd, true
	}

	return m, nil, false
}

func (m pickerModel) commitAction() (tea.Model, tea.Cmd) {
	switch m.action {
	case actionBrowse:
	case actionDelete:
		m.commitDelete()
	case actionRename:
		m.beginRename()
	case actionNew:
		m.beginNew()
	case actionWake:
		err := m.wakeupSession()
		m.exitMode()
		m.applyActionResult(err)
	case actionSleep:
		err := m.sleepSession()
		m.exitMode()
		m.applyActionResult(err)
	}

	return m, nil
}

func (m *pickerModel) beginRename() {
	row, ok := m.currentRow()
	if !ok {
		m.exitMode()

		return
	}

	if row.target.WindowIndex != nil {
		m.renameCurrentWindow()
	} else {
		m.renameCurrentSession()
	}
}

func (m *pickerModel) beginNew() {
	row, ok := m.currentRow()
	if !ok {
		m.exitMode()

		return
	}

	if row.synthetic {
		m.newSession()
	} else {
		m.newWindow()
	}
}

func (m *pickerModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	m.viewport.SetWidth(m.contentWidth())

	reserved := chromeRowsAboveList + 1 + m.statusHeight()
	m.viewport.SetHeight(max(1, m.height-reserved))
}

func (m *pickerModel) applyFilter() {
	query := ""
	if !m.palette {
		query = strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	}

	rows := filteredTreeRows(m.modeSessions(), query, m.windowSort, m.spinnerFrame)
	m.visible = m.decorateRows(rows)

	if len(m.visible) == 0 {
		m.cursor = 0
		m.viewport.SetContent("")

		return
	}

	if m.cursor < 0 || m.cursor >= len(m.visible) || !m.rowSelectable(m.visible[m.cursor]) {
		m.cursor = m.nearestSelectable(m.cursor)
	}
}

func (m pickerModel) nearestSelectable(from int) int {
	if len(m.visible) == 0 {
		return 0
	}

	from = min(max(from, 0), len(m.visible)-1)

	for i := from; i >= 0; i-- {
		if m.rowSelectable(m.visible[i]) {
			return i
		}
	}

	for i := from + 1; i < len(m.visible); i++ {
		if m.rowSelectable(m.visible[i]) {
			return i
		}
	}

	return 0
}

func (m *pickerModel) renderViewport() {
	if len(m.visible) == 0 {
		m.viewport.SetContent("")

		return
	}

	layout := buildPickerTableLayout(m.tableContentWidth())
	barWidth := max(0, m.contentWidth()-2)
	lines := make([]string, 0, len(m.visible))

	for rowIndex, row := range m.visible {
		if rowIndex == m.cursor && m.rowSelectable(row) {
			body := m.selectedMarker(row) + layout.selectedRow(row, m.theme)

			if pad := barWidth - displayWidth(body); pad > 0 {
				body += m.theme.selBar.Render(strings.Repeat(" ", pad))
			}

			lines = append(lines, m.theme.stripe.Render("▌")+m.theme.selBar.Render(" ")+body)

			continue
		}

		lines = append(lines, "  "+m.markerFor(row)+layout.styledRow(row, m.theme))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m pickerModel) markGlyph(row pickerRow) (string, bool) {
	if m.action != actionDelete || row.synthetic {
		return "", false
	}

	switch m.markState(row) {
	case markFull:
		return "●", true
	case markPartial:
		return "◐", true
	default:
		return "○", true
	}
}

func (m pickerModel) markerFor(row pickerRow) string {
	glyph, ok := m.markGlyph(row)
	if !ok {
		if m.markerWidth() > 0 {
			return strings.Repeat(" ", m.markerWidth())
		}

		return ""
	}

	return m.theme.mark.Render(glyph) + " "
}

func (m pickerModel) selectedMarker(row pickerRow) string {
	glyph, ok := m.markGlyph(row)
	if !ok {
		if m.markerWidth() > 0 {
			return m.theme.selBar.Render(strings.Repeat(" ", m.markerWidth()))
		}

		return ""
	}

	return m.theme.markStyle(true).Render(glyph) + m.theme.selBar.Render(" ")
}

func (m pickerModel) markerWidth() int {
	if m.action == actionDelete {
		return 2
	}

	return 0
}

func (m *pickerModel) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}

	return max(1, width-frameChromeWidth)
}

func (m *pickerModel) tableContentWidth() int {
	return max(1, m.contentWidth()-2-m.markerWidth())
}

func (m *pickerModel) rowAtY(mouseY int) (int, bool) {
	rowStart := chromeRowsAboveList + m.statusHeight()

	if mouseY < rowStart {
		return 0, false
	}

	idx := m.viewport.YOffset() + (mouseY - rowStart)
	if idx < 0 || idx >= len(m.visible) || !m.rowSelectable(m.visible[idx]) {
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

//nolint:gochecknoglobals // test seam: picker tests inject a fake tea runner
var newPickerRunner = func(m pickerModel) pickerRunner {
	return tea.NewProgram(m)
}

func ChooseTarget(sessions []Session, windowSort []WindowSortKey, actions Actions) (Target, error) {
	return ChooseTargetWithTheme(sessions, windowSort, actions, themeDark)
}

func ChooseTargetWithTheme(
	sessions []Session,
	windowSort []WindowSortKey,
	actions Actions,
	themeName string,
) (Target, error) {
	if tuiDisabled() {
		return Target{}, errTUIDisabled
	}

	m := newPickerModelWithTheme(sessions, windowSort, actions, themeName)
	runner := newPickerRunner(m)

	finalModel, err := runner.Run()
	if err != nil {
		return Target{}, fmt.Errorf("run picker: %w", err)
	}

	result, ok := finalModel.(pickerModel)
	if !ok {
		return Target{}, errUnexpectedModel
	}

	if result.cancelled {
		return Target{}, errSelectionCanceled
	}

	if strings.TrimSpace(result.selected.SessionName) == "" {
		return Target{}, errNoSessionSelected
	}

	return result.selected, nil
}
