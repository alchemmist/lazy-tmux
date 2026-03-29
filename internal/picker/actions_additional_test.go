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
	model := baseModelForTests()
	model.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	model.cursor = 0
	model.confirmDeleteSession()

	if model.mode != modeConfirmDeleteSession {
		t.Fatalf("expected modeConfirmDeleteSession, got %v", model.mode)
	}

	if !strings.Contains(model.promptInput.Prompt, "demo") {
		t.Fatalf("prompt must mention session name, got %s", model.promptInput.Prompt)
	}
}

func TestRenameCurrentWindowSetsPrompt(t *testing.T) {
	model := baseModelForTests()
	model.visible = []pickerRow{
		{
			target:     Target{SessionName: "demo", WindowIndex: new(2)},
			windowName: "logs",
			selectable: true,
		},
	}
	model.cursor = 0
	model.renameCurrentWindow()

	if model.mode != modeRenameWindow {
		t.Fatalf("expected rename mode, got %v", model.mode)
	}

	if model.pending.WindowIndex == nil || *model.pending.WindowIndex != 2 {
		t.Fatalf("unexpected pending window index: %+v", model.pending.WindowIndex)
	}

	if model.promptInput.Value() != "logs" {
		t.Fatalf("expected prompt value, got %q", model.promptInput.Value())
	}
}

func TestRenameCurrentSessionSetsPrompt(t *testing.T) {
	model := baseModelForTests()
	model.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	model.cursor = 0
	model.renameCurrentSession()

	if model.mode != modeRenameSession {
		t.Fatalf("expected session rename mode, got %v", model.mode)
	}

	if model.promptInput.Value() != "demo" {
		t.Fatalf("expected prompt value to be session name, got %q", model.promptInput.Value())
	}
}

func TestNewSessionSetsMode(t *testing.T) {
	model := baseModelForTests()
	model.newSession()

	if model.mode != modeNewSession {
		t.Fatalf("expected new session mode, got %v", model.mode)
	}

	if !strings.Contains(model.promptInput.Prompt, "New session") {
		t.Fatalf("unexpected prompt: %s", model.promptInput.Prompt)
	}
}

func TestNewWindowRequiresSessionSelection(t *testing.T) {
	model := baseModelForTests()
	model.newWindow()

	if !strings.Contains(model.statusMsg, "select a session") {
		t.Fatalf("expected status message, got %q", model.statusMsg)
	}
}

func TestNewWindowPreparesPrompt(t *testing.T) {
	model := baseModelForTests()
	model.visible = []pickerRow{{target: Target{SessionName: "demo"}, selectable: true}}
	model.cursor = 0
	model.newWindow()

	if model.mode != modeNewWindow {
		t.Fatalf("expected new window mode, got %v", model.mode)
	}

	if !strings.Contains(model.promptInput.Prompt, "demo") {
		t.Fatalf("prompt should mention demo, got %s", model.promptInput.Prompt)
	}
}

func TestHandlePromptKeyCreatesSession(t *testing.T) {
	created := false
	model := baseModelForTests()
	model.mode = modeNewSession
	model.promptInput.SetValue("demo")
	model.actions.NewSession = func(name string) error {
		created = true

		if name != "demo" {
			t.Fatalf("unexpected session %q", name)
		}

		return nil
	}
	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	model := baseModelForTests()
	model.mode = modeRenameWindow
	model.pending = Target{SessionName: "demo", WindowIndex: ptr(1)}
	model.promptInput.SetValue("new")
	model.actions.RenameWindow = func(session string, windowIndex int, name string) error {
		renamed = true

		if session != "demo" || windowIndex != 1 || name != "new" {
			t.Fatalf("unexpected args %s %d %s", session, windowIndex, name)
		}

		return nil
	}
	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	model := baseModelForTests()
	model.mode = modeRenameSession
	model.pending = Target{SessionName: "demo"}
	model.promptInput.SetValue("new")
	model.actions.RenameSession = func(session, name string) error {
		renamed = true

		if session != "demo" || name != "new" {
			t.Fatalf("unexpected args %s %s", session, name)
		}

		return nil
	}
	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	model := baseModelForTests()
	model.mode = modeNewWindow
	model.pending = Target{SessionName: "demo"}
	model.promptInput.SetValue("win")
	model.actions.NewWindow = func(session, name string) error {
		created = true

		if session != "demo" || name != "win" {
			t.Fatalf("unexpected args %s %s", session, name)
		}

		return nil
	}
	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !created {
		t.Fatal("expected new window called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected browse mode, got %v", out.mode)
	}
}

func TestSetStatusTrimmedAndCleared(t *testing.T) {
	model := baseModelForTests()
	model.setStatus("  msg ")

	if model.statusMsg != "msg" {
		t.Fatalf("unexpected status %q", model.statusMsg)
	}

	if model.statusHeight() != 1 {
		t.Fatalf("expected height 1, got %d", model.statusHeight())
	}

	model.clearStatus()

	if model.statusMsg != "" {
		t.Fatalf("status should be empty, got %q", model.statusMsg)
	}

	if model.statusHeight() != 0 {
		t.Fatalf("expected height 0, got %d", model.statusHeight())
	}
}
