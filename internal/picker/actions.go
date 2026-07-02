//go:build !lazy_fzf

package picker

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Sentinel errors surfaced in the picker's status line.
var (
	errDeleteWindowUnavailable  = errors.New("delete window not available")
	errDeleteSessionUnavailable = errors.New("delete session not available")
	errSelectSessionToDelete    = errors.New("select a session to delete")
	errRenameWindowUnavailable  = errors.New("rename window not available")
	errRenameSessionUnavailable = errors.New("rename session not available")
	errNewSessionUnavailable    = errors.New("new session not available")
	errNewWindowUnavailable     = errors.New("new window not available")
	errSelectSessionForWindow   = errors.New("select a session to create a window")
	errSelectSessionToWakeup    = errors.New("select a session to wakeup")
	errWakeupUnavailable        = errors.New("wakeup not available")
	errSelectSessionToSleep     = errors.New("select a session to sleep")
	errSleepUnavailable         = errors.New("sleep not available")
)

// markState describes how much of a row is marked for deletion: a window is
// either marked or not; a session is empty, partial or fully marked.
type markState int

const (
	markNone markState = iota
	markPartial
	markFull
)

// targetKey identifies a window in the mark set; the NUL byte cannot appear in a
// session name, so it is an unambiguous separator.
func targetKey(session string, windowIndex int) string {
	return session + "\x00" + strconv.Itoa(windowIndex)
}

// parseTargetKey splits a mark-set key back into session name and window index.
func parseTargetKey(key string) (string, int, bool) {
	session, rest, found := strings.Cut(key, "\x00")
	if !found {
		return "", 0, false
	}

	idx, err := strconv.Atoi(rest)
	if err != nil {
		return "", 0, false
	}

	return session, idx, true
}

func (m *pickerModel) sessionWindowIndices(session string) []int {
	for index := range m.sessions {
		if m.sessions[index].Record.SessionName != session {
			continue
		}

		idxs := make([]int, 0, len(m.sessions[index].Windows))
		for _, win := range m.sessions[index].Windows {
			idxs = append(idxs, win.Index)
		}

		return idxs
	}

	return nil
}

// markWindow marks a single window for deletion.
func (m *pickerModel) markWindow(target Target) {
	if target.WindowIndex == nil {
		return
	}

	m.marked[targetKey(target.SessionName, *target.WindowIndex)] = struct{}{}
}

// markSession toggles every window of a session: if all are already marked it
// clears them, otherwise it marks them all (so a fully-marked session deletes).
func (m *pickerModel) markSession(session string) {
	idxs := m.sessionWindowIndices(session)
	if len(idxs) == 0 {
		return
	}

	unmarkAll := true

	for _, idx := range idxs {
		if _, ok := m.marked[targetKey(session, idx)]; !ok {
			unmarkAll = false

			break
		}
	}

	for _, idx := range idxs {
		key := targetKey(session, idx)
		if unmarkAll {
			delete(m.marked, key)
		} else {
			m.marked[key] = struct{}{}
		}
	}
}

// toggleMark flips the mark for the row under the cursor (window: itself;
// session header: all of its windows).
func (m *pickerModel) toggleMark(row pickerRow) {
	if row.synthetic {
		return
	}

	if row.target.WindowIndex == nil {
		m.markSession(row.target.SessionName)

		return
	}

	key := targetKey(row.target.SessionName, *row.target.WindowIndex)
	if _, ok := m.marked[key]; ok {
		delete(m.marked, key)
	} else {
		m.marked[key] = struct{}{}
	}
}

func (m pickerModel) markState(row pickerRow) markState {
	if row.target.WindowIndex != nil {
		if _, ok := m.marked[targetKey(row.target.SessionName, *row.target.WindowIndex)]; ok {
			return markFull
		}

		return markNone
	}

	idxs := m.sessionWindowIndices(row.target.SessionName)
	if len(idxs) == 0 {
		return markNone
	}

	marked := 0

	for _, idx := range idxs {
		if _, ok := m.marked[targetKey(row.target.SessionName, idx)]; ok {
			marked++
		}
	}

	switch {
	case marked == 0:
		return markNone
	case marked == len(idxs):
		return markFull
	default:
		return markPartial
	}
}

// commitDelete removes every marked window, collapsing a fully-marked session
// into a single DeleteSession. Windows are removed high-index-first so earlier
// indices stay valid, and sessions are processed in a stable order.
func (m *pickerModel) commitDelete() {
	if len(m.marked) == 0 {
		m.exitMode()

		return
	}

	bySession := map[string][]int{}

	for key := range m.marked {
		session, idx, ok := parseTargetKey(key)
		if !ok {
			continue
		}

		bySession[session] = append(bySession[session], idx)
	}

	sessions := make([]string, 0, len(bySession))
	for session := range bySession {
		sessions = append(sessions, session)
	}

	sort.Strings(sessions)

	var firstErr error

	for _, session := range sessions {
		err := m.deleteMarkedSession(session, bySession[session])
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		m.setStatus(firstErr.Error())
	} else {
		m.clearStatus()
	}

	m.exitMode()
	m.reload()
	m.renderViewport()
}

func (m *pickerModel) deleteMarkedSession(session string, idxs []int) error {
	if total := len(m.sessionWindowIndices(session)); total > 0 && len(idxs) >= total {
		return m.deleteSession(session)
	}

	if m.actions.DeleteWindow == nil {
		return errDeleteWindowUnavailable
	}

	sort.Sort(sort.Reverse(sort.IntSlice(idxs)))

	var firstErr error

	for _, idx := range idxs {
		err := m.actions.DeleteWindow(session, idx)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
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
	m.pending = Target{}
	m.mode = modeNewSession
	m.promptInput = textinput.New()
	m.promptInput.Prompt = "New session name: "
	m.promptInput.Focus()
	m.resize()
}

func (m *pickerModel) newWindow() {
	row, ok := m.currentRow()
	if !ok || strings.TrimSpace(row.target.SessionName) == "" {
		m.setStatus("select a session to create a window")

		return
	}

	m.pending = row.target
	m.mode = modeNewWindow
	m.promptInput = textinput.New()
	m.promptInput.Prompt = fmt.Sprintf("New window in %s: ", row.target.SessionName)
	m.promptInput.Focus()
	m.resize()
}

func (m pickerModel) handlePromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc, keyCtrlC:
		m.promptInput.Blur()
		m.mode = modeBrowse
		m.exitMode() // cancel the prompt and its action mode, back to browse

		return m, nil
	case keyEnter:
		m.commitPrompt()

		m.promptInput.Blur()
		m.mode = modeBrowse
		m.exitMode() // action completed, return to browse (orange)

		return m, nil
	}

	var cmd tea.Cmd

	m.promptInput, cmd = m.promptInput.Update(msg)

	return m, cmd
}

// commitPrompt runs the action of the active text-prompt mode with the typed
// name, then refreshes the table. Prompts that require a name are a no-op while
// the input is empty (only the new-window prompt allows an empty name: tmux
// then picks the default window name).
func (m *pickerModel) commitPrompt() {
	name := strings.TrimSpace(m.promptInput.Value())

	var (
		err error
		ran bool
	)

	switch m.mode {
	case modeBrowse:
		return
	case modeRenameWindow:
		if name != "" && m.pending.WindowIndex != nil {
			err = m.renameWindow(m.pending.SessionName, *m.pending.WindowIndex, name)
			ran = true
		}
	case modeRenameSession:
		if name != "" {
			err = m.renameSession(m.pending.SessionName, name)
			ran = true
		}
	case modeNewSession:
		if name != "" {
			err = m.createSession(name)
			ran = true
		}
	case modeNewWindow:
		err = m.createWindow(m.pending.SessionName, name)
		ran = true
	}

	if !ran {
		return
	}

	if err != nil {
		m.setStatus(err.Error())
	} else {
		m.clearStatus()
	}

	m.reload()
	m.renderViewport()
}

func (m *pickerModel) deleteSession(session string) error {
	if m.actions.DeleteSession == nil {
		return errDeleteSessionUnavailable
	}

	if strings.TrimSpace(session) == "" {
		return errSelectSessionToDelete
	}

	return m.actions.DeleteSession(session)
}

func (m *pickerModel) renameWindow(session string, windowIndex int, name string) error {
	if m.actions.RenameWindow == nil {
		return errRenameWindowUnavailable
	}

	return m.actions.RenameWindow(session, windowIndex, name)
}

func (m *pickerModel) renameSession(session, name string) error {
	if m.actions.RenameSession == nil {
		return errRenameSessionUnavailable
	}

	return m.actions.RenameSession(session, name)
}

func (m *pickerModel) createSession(name string) error {
	if m.actions.NewSession == nil {
		return errNewSessionUnavailable
	}

	return m.actions.NewSession(name)
}

func (m *pickerModel) createWindow(session, name string) error {
	if m.actions.NewWindow == nil {
		return errNewWindowUnavailable
	}

	if strings.TrimSpace(session) == "" {
		return errSelectSessionForWindow
	}

	return m.actions.NewWindow(session, name)
}

func (m *pickerModel) wakeupSession() error {
	row, ok := m.currentRow()
	if !ok {
		return errSelectSessionToWakeup
	}

	if m.actions.Wakeup == nil {
		return errWakeupUnavailable
	}

	return m.actions.Wakeup(row.target.SessionName)
}

func (m *pickerModel) sleepSession() error {
	row, ok := m.currentRow()
	if !ok {
		return errSelectSessionToSleep
	}

	if m.actions.Sleep == nil {
		return errSleepUnavailable
	}

	return m.actions.Sleep(row.target.SessionName)
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

func (m *pickerModel) moveNextSelectable() {
	if len(m.visible) == 0 {
		return
	}

	for i := m.cursor + 1; i < len(m.visible); i++ {
		if m.rowSelectable(m.visible[i]) {
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
		if m.rowSelectable(m.visible[i]) {
			m.cursor = i

			return
		}
	}
}
