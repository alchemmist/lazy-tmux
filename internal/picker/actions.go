//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

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
	case "esc", "ctrl+c":
		m.mode = modeBrowse
		m.promptInput.Blur()
		m.resize()

		return m, nil
	case "enter":
		switch m.mode {
		case modeConfirmDeleteSession:
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
		case modeRenameWindow:
			name := strings.TrimSpace(m.promptInput.Value())
			if name != "" && m.pending.WindowIndex != nil {
				if err := m.renameWindow(
					m.pending.SessionName,
					*m.pending.WindowIndex,
					name,
				); err != nil {
					m.setStatus(err.Error())
				} else {
					m.clearStatus()
				}

				m.reload()
				m.renderViewport()
			}
		case modeRenameSession:
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
		case modeNewSession:
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
		case modeNewWindow:
			name := strings.TrimSpace(m.promptInput.Value())
			if err := m.createWindow(m.pending.SessionName, name); err != nil {
				m.setStatus(err.Error())
			} else {
				m.clearStatus()
			}

			m.reload()
			m.renderViewport()
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

func (m *pickerModel) renameSession(session, name string) error {
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

func (m *pickerModel) createWindow(session, name string) error {
	if m.actions.NewWindow == nil {
		return fmt.Errorf("new window not available")
	}

	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("select a session to create a window")
	}

	return m.actions.NewWindow(session, name)
}

func (m *pickerModel) wakeupSession() error {
	row, ok := m.currentRow()
	if !ok {
		return fmt.Errorf("select a session to wakeup")
	}

	if m.actions.Wakeup == nil {
		return fmt.Errorf("wakeup not available")
	}

	return m.actions.Wakeup(row.target.SessionName)
}

func (m *pickerModel) sleepSession() error {
	row, ok := m.currentRow()
	if !ok {
		return fmt.Errorf("select a session to sleep")
	}

	if m.actions.Sleep == nil {
		return fmt.Errorf("sleep not available")
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

func nearestSelectableRow(rows []pickerRow, from int) int {
	if len(rows) == 0 {
		return 0
	}

	if from < 0 {
		from = 0
	}

	if from >= len(rows) {
		from = len(rows) - 1
	}

	for i := from; i >= 0; i-- {
		if rows[i].selectable {
			return i
		}
	}

	for i := from + 1; i < len(rows); i++ {
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
