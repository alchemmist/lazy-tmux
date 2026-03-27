//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func baseModelForTests() pickerModel {
	return pickerModel{
		viewport:    viewport.New(),
		width:       80,
		height:      20,
		queryInput:  textinput.New(),
		promptInput: textinput.New(),
	}
}

func TestConfirmDeleteSessionPreparesPrompt(t *testing.T) {
	m := baseModelForTests()
	m.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	m.cursor = 0
	m.confirmDeleteSession()

	if m.mode != modeConfirmDeleteSession {
		t.Fatalf("expected modeConfirmDeleteSession, got %v", m.mode)
	}

	if !strings.Contains(m.promptInput.Prompt, "demo") {
		t.Fatalf("prompt must mention session name, got %s", m.promptInput.Prompt)
	}
}

func TestRenameCurrentWindowSetsPrompt(t *testing.T) {
	m := baseModelForTests()
	m.visible = []pickerRow{{target: Target{SessionName: "demo", WindowIndex: ptr(2)}, windowName: "logs", selectable: true}}
	m.cursor = 0
	m.renameCurrentWindow()

	if m.mode != modeRenameWindow {
		t.Fatalf("expected rename mode, got %v", m.mode)
	}

	if m.pending.WindowIndex == nil || *m.pending.WindowIndex != 2 {
		t.Fatalf("unexpected pending window index: %+v", m.pending.WindowIndex)
	}

	if m.promptInput.Value() != "logs" {
		t.Fatalf("expected prompt value, got %q", m.promptInput.Value())
	}
}

func TestRenameCurrentSessionSetsPrompt(t *testing.T) {
	m := baseModelForTests()
	m.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	m.cursor = 0
	m.renameCurrentSession()

	if m.mode != modeRenameSession {
		t.Fatalf("expected session rename mode, got %v", m.mode)
	}

	if m.promptInput.Value() != "demo" {
		t.Fatalf("expected prompt value to be session name, got %q", m.promptInput.Value())
	}
}

func TestNewSessionSetsMode(t *testing.T) {
	m := baseModelForTests()
	m.newSession()

	if m.mode != modeNewSession {
		t.Fatalf("expected new session mode, got %v", m.mode)
	}

	if !strings.Contains(m.promptInput.Prompt, "New session") {
		t.Fatalf("unexpected prompt: %s", m.promptInput.Prompt)
	}
}

func TestNewWindowRequiresSessionSelection(t *testing.T) {
	m := baseModelForTests()
	m.newWindow()

	if !strings.Contains(m.statusMsg, "select a session") {
		t.Fatalf("expected status message, got %q", m.statusMsg)
	}
}

func TestNewWindowPreparesPrompt(t *testing.T) {
	m := baseModelForTests()
	m.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	m.cursor = 0
	m.newWindow()

	if m.mode != modeNewWindow {
		t.Fatalf("expected new window mode, got %v", m.mode)
	}

	if !strings.Contains(m.promptInput.Prompt, "demo") {
		t.Fatalf("prompt should mention demo, got %s", m.promptInput.Prompt)
	}
}

func TestHandlePromptKeyCreatesSession(t *testing.T) {
	created := false
	m := baseModelForTests()
	m.mode = modeNewSession
	m.promptInput.SetValue("demo")
	m.actions.NewSession = func(name string) error {
		created = true

		if name != "demo" {
			t.Fatalf("unexpected session %q", name)
		}

		return nil
	}
	next, _ := m.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !created {
		t.Fatal("expected create session to be called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected browse mode, got %v", out.mode)
	}
}

func TestHandlePromptKeyRenamesWindow(t *testing.T) {
	renamed := false
	m := baseModelForTests()
	m.mode = modeRenameWindow
	m.pending = Target{SessionName: "demo", WindowIndex: ptr(1)}
	m.promptInput.SetValue("new")
	m.actions.RenameWindow = func(session string, windowIndex int, name string) error {
		renamed = true

		if session != "demo" || windowIndex != 1 || name != "new" {
			t.Fatalf("unexpected args %s %d %s", session, windowIndex, name)
		}

		return nil
	}
	next, _ := m.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !renamed {
		t.Fatal("expected rename window called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected browse mode, got %v", out.mode)
	}
}

func TestHandlePromptKeyRenamesSession(t *testing.T) {
	renamed := false
	m := baseModelForTests()
	m.mode = modeRenameSession
	m.pending = Target{SessionName: "demo"}
	m.promptInput.SetValue("new")
	m.actions.RenameSession = func(session, name string) error {
		renamed = true

		if session != "demo" || name != "new" {
			t.Fatalf("unexpected args %s %s", session, name)
		}

		return nil
	}
	next, _ := m.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !renamed {
		t.Fatal("expected rename session called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected browse mode, got %v", out.mode)
	}
}

func TestHandlePromptKeyCreatesWindow(t *testing.T) {
	created := false
	m := baseModelForTests()
	m.mode = modeNewWindow
	m.pending = Target{SessionName: "demo"}
	m.promptInput.SetValue("win")
	m.actions.NewWindow = func(session, name string) error {
		created = true

		if session != "demo" || name != "win" {
			t.Fatalf("unexpected args %s %s", session, name)
		}

		return nil
	}
	next, _ := m.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !created {
		t.Fatal("expected new window called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected browse mode, got %v", out.mode)
	}
}

func TestSetStatusTrimmedAndCleared(t *testing.T) {
	m := baseModelForTests()
	m.setStatus("  msg ")

	if m.statusMsg != "msg" {
		t.Fatalf("unexpected status %q", m.statusMsg)
	}

	if m.statusHeight() != 1 {
		t.Fatalf("expected height 1, got %d", m.statusHeight())
	}

	m.clearStatus()

	if m.statusMsg != "" {
		t.Fatalf("status should be empty, got %q", m.statusMsg)
	}

	if m.statusHeight() != 0 {
		t.Fatalf("expected height 0, got %d", m.statusHeight())
	}
}
