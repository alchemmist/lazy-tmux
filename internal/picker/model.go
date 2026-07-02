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

// statusRefreshInterval is how often the picker re-reads live window statuses
// (e.g. Claude working/idle) so the dots update while it stays open.
const statusRefreshInterval = 2 * time.Second

// statusTickMsg drives the periodic live-status refresh.
type statusTickMsg struct{}

type pickerRow struct {
	target     Target
	item       string
	captured   string
	wins       string
	state      string
	cmd        string
	windowName string
	status     WindowStatus // live program status (window rows); drives the State dot
	selectable bool         // inherent browse-mode selectability (window rows)
	synthetic  bool         // the "＋ new session" row injected in new mode
}

// pickerModel deliberately mixes receiver kinds: bubbletea's Elm architecture
// requires value receivers for the tea.Model interface (Init/Update/View),
// while internal helpers mutate the model through pointer receivers before it
// is returned.
//
//nolint:recvcheck // deliberate value/pointer receiver mix, see above
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
	action      actionMode
	marked      map[string]struct{} // delete multi-select, keyed by targetKey
	palette     bool                // slash-command entry is open
	paletteIdx  int                 // highlighted command in the palette
	promptInput textinput.Model
	pending     Target
}

// pickerMode is the text-prompt overlay (rename/new). It is orthogonal to the
// colored actionMode: a mode stays active while its prompt collects input.
type pickerMode int

const (
	modeBrowse pickerMode = iota
	modeRenameWindow
	modeRenameSession
	modeNewSession
	modeNewWindow
)

const scrollMargin = 2

func newPickerModel(sessions []Session, windowSort []WindowSortKey, actions Actions) pickerModel {
	theme := newPickerTheme(colAccent)

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
		action:     actionBrowse,
		marked:     make(map[string]struct{}),
	}
	model.applyFilter()

	return model
}

func (m pickerModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, scheduleStatusRefresh())
}

func scheduleStatusRefresh() tea.Cmd {
	return tea.Tick(statusRefreshInterval, func(time.Time) tea.Msg { return statusTickMsg{} })
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
		// Refresh live window statuses while browsing; never disturb an active
		// prompt or palette. Always re-arm the ticker.
		if m.mode == modeBrowse && !m.palette {
			m.reload()
			m.renderViewport()
		}

		return m, scheduleStatusRefresh()
	case tea.MouseWheelMsg:
		if m.mode != modeBrowse || m.palette {
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
		if m.mode != modeBrowse || m.palette || msg.Button != tea.MouseLeft {
			return m, nil
		}

		idx, ok := m.rowAtY(msg.Y)
		if !ok {
			return m, nil
		}

		return m.handleRowClick(idx)
	case tea.KeyPressMsg:
		if m.mode != modeBrowse {
			return m.handlePromptKey(msg)
		}

		if m.palette {
			if next, handled := m.handlePaletteKey(msg); handled {
				return next, nil
			}
		} else if m.action != actionBrowse {
			if next, cmd, handled := m.handleActionKey(msg); handled {
				return next, cmd
			}
		} else if next, cmd, handled := m.handleBrowseKey(msg); handled {
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

	// Top border: title (with the active mode) + session/window counts.
	sessions, windows := m.counts()
	right := m.theme.count.Render(fmt.Sprintf("%d sessions · %d windows", sessions, windows))
	buf.WriteString(m.theme.frameTop(m.titleText(), right, width))
	buf.WriteString("\n")

	// Search input (or the active text prompt for rename/new).
	input := m.queryInput.View()
	if m.mode != modeBrowse {
		input = m.promptInput.View()
	}

	buf.WriteString(m.theme.frameLine(input, width))
	buf.WriteString("\n")

	if m.palette {
		m.writePalette(&buf, width)
	} else {
		m.writeTable(&buf, width)
	}

	// Bottom border: key hints.
	buf.WriteString(m.theme.frameBottom(m.helpHints(), width))

	view := tea.NewView(buf.String())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion

	return view
}

// titleText is "lazy-tmux" while browsing, or "lazy-tmux · DELETE" in a mode.
func (m pickerModel) titleText() string {
	if cmd, ok := commandForMode(m.action); ok {
		return "lazy-tmux · " + cmd.label
	}

	return "lazy-tmux"
}

// writeTable renders the column header, optional status line and the row
// viewport (or an empty-state message) into buf.
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

// writePalette renders the slash-command dropdown into buf, highlighting the
// selected command with the browse-accent stripe.
func (m pickerModel) writePalette(buf *strings.Builder, width int) {
	matches := matchCommands(m.commandPrefix())
	if len(matches) == 0 {
		buf.WriteString(m.theme.frameLine(m.theme.faint.Render("no matching command"), width))
		buf.WriteString("\n")

		return
	}

	for i, cmd := range matches {
		name := m.theme.title.Render("/" + cmd.name)
		line := name + "  " + m.theme.meta.Render(cmd.desc)

		if i == m.paletteIdx {
			line = m.theme.stripe.Render("▌") + " " + line
		} else {
			line = "  " + line
		}

		buf.WriteString(m.theme.frameLine(line, width))
		buf.WriteString("\n")
	}
}

// handleRowClick performs the click action for a row, per the active mode:
// browse selects (and quits when re-clicking the cursor row), delete toggles
// the mark, and the single-target modes act on the clicked row.
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

// syncPalette opens or closes the slash-command palette based on the current
// query: a leading "/" (while browsing) enters command entry; anything else
// leaves it. paletteIdx is clamped to the available matches.
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

// commandPrefix is the typed command name (the query text after the "/").
func (m pickerModel) commandPrefix() string {
	return strings.TrimPrefix(m.queryInput.Value(), "/")
}

// counts returns the number of sessions and the total number of windows.
func (m pickerModel) counts() (int, int) {
	windows := 0
	for i := range m.sessions {
		windows += len(m.sessions[i].Windows)
	}

	return len(m.sessions), windows
}

func (m pickerModel) helpHints() string {
	var pairs []hint

	switch {
	case m.palette:
		pairs = []hint{{"↵", "run"}, {"tab", "complete"}, {"^j/^k", "move"}, {"esc", "cancel"}}
	case m.action != actionBrowse:
		cmd, _ := commandForMode(m.action)
		pairs = append([]hint{{cmd.label, ""}}, cmd.hints...)
		pairs = append(pairs, hint{"^j/^k", "move"}, hint{"esc", "cancel"})
	default:
		pairs = []hint{
			{"↵", "select"}, {"^j/^k", "move"}, {"/", "commands"}, {"esc", "quit"},
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

// handleBrowseKey dispatches a key press while browsing. The bool reports
// whether the key was consumed; unconsumed keys fall through to the search
// input so typing still filters the list.
func (m pickerModel) handleBrowseKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q", "esc":
		m.cancelled = true

		return m, tea.Quit, true
	case "ctrl+d", "alt+d", "ctrl+r", "alt+r", "ctrl+n", "alt+n", "alt+w", "alt+s":
		m.enterModeShortcut(msg.String())
		m.renderViewport()

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

// enterMode switches into a colored action mode: it rebuilds the theme around
// the mode accent, clears any query/marks, and re-filters the list to the
// mode's valid targets.
func (m *pickerModel) enterMode(mode actionMode) {
	m.action = mode
	m.palette = false
	m.paletteIdx = 0
	m.marked = make(map[string]struct{})
	m.queryInput.SetValue("")
	m.theme = newPickerTheme(accentForMode(mode))
	m.resize()
	m.applyFilter() // clamps the carried-over cursor to a mode-selectable row
	m.ensureCursorVisible()
}

// enterModeShortcut maps a legacy chord to its mode, positioning the cursor on
// the relevant row (and pre-marking it in delete mode) so muscle memory keeps
// working: ^d/^r target the current window, alt+d/alt+r the current session,
// ^n adds a window to it, alt+n starts a fresh session, alt+w/alt+s the session.
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

// focusWindow moves the cursor onto a specific window row, if visible.
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

// focusSession moves the cursor onto a session header row, if visible.
func (m *pickerModel) focusSession(name string) {
	for i, row := range m.visible {
		if !row.synthetic && row.target.WindowIndex == nil && row.target.SessionName == name {
			m.cursor = i
			m.ensureCursorVisible()

			return
		}
	}
}

// focusSynthetic moves the cursor onto the synthetic "＋ new session" row.
func (m *pickerModel) focusSynthetic() {
	for i, row := range m.visible {
		if row.synthetic {
			m.cursor = i
			m.ensureCursorVisible()

			return
		}
	}
}

// exitMode returns to the resting browse (orange) mode, dropping the action,
// palette and marks and restoring the full list. It keeps the cursor on the row
// that was acted on (so Esc lands you back where you were) when that row still
// exists.
func (m *pickerModel) exitMode() {
	prev, hadRow := m.currentRow()

	m.action = actionBrowse
	m.palette = false
	m.paletteIdx = 0
	m.marked = make(map[string]struct{})
	m.queryInput.SetValue("")
	m.theme = newPickerTheme(colAccent)
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

// handlePaletteKey drives the slash-command dropdown. Printable keys fall
// through (handled=false) so the typed command name keeps editing the query.
func (m pickerModel) handlePaletteKey(msg tea.KeyPressMsg) (tea.Model, bool) {
	matches := matchCommands(m.commandPrefix())

	switch msg.String() {
	case "esc", "ctrl+c":
		m.exitMode()

		return m, true
	case "ctrl+k":
		if m.paletteIdx > 0 {
			m.paletteIdx--
		}

		return m, true
	case "ctrl+j":
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
	case "enter":
		if len(matches) > 0 {
			m.enterMode(matches[m.paletteIdx].mode)
			m.renderViewport()
		}

		return m, true
	}

	return m, false
}

// handleActionKey drives an active action mode. Unhandled printable keys fall
// through so typing still filters the mode's list.
func (m pickerModel) handleActionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q":
		m.cancelled = true

		return m, tea.Quit, true
	case "esc":
		m.exitMode()

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
	case "space":
		if m.action != actionDelete {
			return m, nil, false
		}

		if row, ok := m.currentRow(); ok {
			m.toggleMark(row)
			m.renderViewport()
		}

		return m, nil, true
	case "enter":
		next, cmd := m.commitAction()

		return next, cmd, true
	}

	return m, nil, false
}

// commitAction performs the active mode's action on the current row: delete
// removes all marked targets; rename/new open a themed prompt; wake/sleep act
// immediately and drop back to browse.
func (m pickerModel) commitAction() (tea.Model, tea.Cmd) {
	switch m.action {
	case actionBrowse:
		// Enter in browse selects a row; commitAction is never reached.
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

// beginRename opens the rename prompt for the picked row (window or session),
// keeping the rename accent until the prompt resolves.
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

// beginNew opens the new-session prompt for the synthetic row, or the
// new-window prompt for a picked session.
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

	// Reserved chrome rows: top border, input, header, bottom border (+ status).
	reserved := 4 + m.statusHeight()
	m.viewport.SetHeight(max(1, m.height-reserved))
}

func (m *pickerModel) applyFilter() {
	query := ""
	if !m.palette {
		query = strings.TrimSpace(strings.ToLower(m.queryInput.Value()))
	}

	rows := filteredTreeRows(m.modeSessions(), query, m.windowSort)
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

// nearestSelectable finds the closest mode-selectable row at or around `from`.
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
	barWidth := max(0, m.contentWidth()-2) // room for the stripe + a space
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

// markGlyph returns the multi-select indicator glyph for a row, and whether one
// applies (only window/session rows in delete mode have one).
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

// markerFor returns the leading multi-select indicator for a non-selected row.
// The width is constant (markerWidth) so columns stay aligned across rows.
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

// selectedMarker is markerFor for the cursor row: the mark glyph keeps its color
// on the selection background, and the gap is filled with the selection bar.
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

// markerWidth is the fixed width reserved for the delete-mode mark column.
func (m pickerModel) markerWidth() int {
	if m.action == actionDelete {
		return 2 // glyph + space
	}

	return 0
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
// leading cells reserved for the selection stripe / row indent and the
// (delete-mode) mark column.
func (m *pickerModel) tableContentWidth() int {
	return max(1, m.contentWidth()-2-m.markerWidth())
}

// rowAtY maps a mouse Y coordinate to a selectable row index. The list starts
// below the top border, input and header (and the optional status line).
func (m *pickerModel) rowAtY(mouseY int) (int, bool) {
	rowStart := 3 + m.statusHeight() // top border + input + header [+ status]

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
	if tuiDisabled() {
		return Target{}, errors.New("TUI picker disabled in fzf-only build")
	}

	m := newPickerModel(sessions, windowSort, actions)
	runner := newPickerRunner(m)

	finalModel, err := runner.Run()
	if err != nil {
		return Target{}, fmt.Errorf("run picker: %w", err)
	}

	result, ok := finalModel.(pickerModel)
	if !ok {
		return Target{}, errors.New("unexpected picker model type")
	}

	if result.cancelled {
		return Target{}, errors.New("selection canceled")
	}

	if strings.TrimSpace(result.selected.SessionName) == "" {
		return Target{}, errors.New("no session selected")
	}

	return result.selected, nil
}
