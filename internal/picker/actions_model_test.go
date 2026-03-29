//go:build !lazy_fzf

package picker

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestNearestSelectableRow(t *testing.T) {
	rows := []pickerRow{
		{item: "session-a", selectable: false},
		{item: "  ├─ [0] editor", selectable: true},
		{item: "  ╰─ [1] logs", selectable: true},
		{item: "session-b", selectable: false},
	}

	if got := nearestSelectableRow(rows, 0); got != 1 {
		t.Fatalf("expected nearest selectable from 0 to be 1, got %d", got)
	}

	if got := nearestSelectableRow(rows, 3); got != 2 {
		t.Fatalf("expected nearest selectable from 3 to be 2, got %d", got)
	}

	if got := nearestSelectableRow(nil, 2); got != 0 {
		t.Fatalf("expected 0 for empty rows, got %d", got)
	}
}

func TestCurrentRowOutOfRange(t *testing.T) {
	model := pickerModel{
		visible: []pickerRow{{item: "ok", selectable: true}},
		cursor:  2,
	}
	if _, ok := model.currentRow(); ok {
		t.Fatal("expected currentRow to fail when cursor is out of range")
	}
}

func TestDeleteSessionValidatesActionAndName(t *testing.T) {
	model := pickerModel{}
	if err := model.deleteSession("demo"); err == nil {
		t.Fatal("expected error when delete action is nil")
	}

	called := false

	model.actions.DeleteSession = func(session string) error {
		called = true
		return nil
	}
	if err := model.deleteSession(" "); err == nil {
		t.Fatal("expected error when session name is empty")
	}

	if called {
		t.Fatal("delete action must not be called on empty session")
	}

	if err := model.deleteSession("demo"); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
}

func TestCreateWindowValidatesActionAndSession(t *testing.T) {
	model := pickerModel{}
	if err := model.createWindow("demo", ""); err == nil {
		t.Fatal("expected error when create window action is nil")
	}

	called := false

	model.actions.NewWindow = func(session, name string) error {
		called = true

		if session == "" {
			t.Fatal("session must not be empty")
		}

		return nil
	}
	if err := model.createWindow(" ", ""); err == nil {
		t.Fatal("expected error when session is empty")
	}

	if called {
		t.Fatal("new window action must not be called on empty session")
	}

	if err := model.createWindow("demo", "win"); err != nil {
		t.Fatalf("unexpected create window error: %v", err)
	}
}

func TestApplyFilterMovesCursorToSelectableRow(t *testing.T) {
	base := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{
			Record: snapshot.Record{SessionName: "work", CapturedAt: base, Windows: 1},
			Windows: []snapshot.Window{
				{Index: 0, Name: "editor"},
			},
		},
	}

	input := textinput.New()
	model := pickerModel{
		sessions:   sessions,
		windowSort: DefaultSortOptions().Window,
		queryInput: input,
		cursor:     0, // session row (non-selectable)
	}
	model.applyFilter()

	if len(model.visible) < 2 {
		t.Fatalf("expected at least 2 rows, got %d", len(model.visible))
	}

	if model.cursor != 1 {
		t.Fatalf("expected cursor to move to selectable row, got %d", model.cursor)
	}
}

func TestDeleteCurrentWindowInvokesAction(t *testing.T) {
	called := false
	model := pickerModel{
		visible: []pickerRow{
			{
				target:     Target{SessionName: "demo", WindowIndex: ptr(2)},
				selectable: true,
			},
		},
		cursor: 0,
		actions: Actions{
			DeleteWindow: func(session string, windowIndex int) error {
				called = true
				if session != "demo" || windowIndex != 2 {
					t.Fatalf("unexpected args: %s %d", session, windowIndex)
				}
				return nil
			},
		},
	}

	if err := model.deleteCurrentWindow(); err != nil {
		t.Fatalf("deleteCurrentWindow error: %v", err)
	}

	if !called {
		t.Fatal("expected DeleteWindow to be called")
	}
}

func TestHandlePromptKeyConfirmDeleteSession(t *testing.T) {
	deleted := false

	sessions := []Session{
		{
			Record: snapshot.Record{SessionName: "demo", CapturedAt: time.Now().UTC(), Windows: 1},
			Windows: []snapshot.Window{
				{Index: 0, Name: "editor"},
			},
		},
	}

	model := pickerModel{
		windowSort: DefaultSortOptions().Window,
		viewport:   viewport.New(),
		width:      80,
		height:     20,
		queryInput: textinput.New(),
		promptInput: func() textinput.Model {
			in := textinput.New()
			in.SetValue("y")
			return in
		}(),
		mode:    modeConfirmDeleteSession,
		pending: Target{SessionName: "demo"},
		actions: Actions{
			DeleteSession: func(session string) error {
				deleted = true
				if session != "demo" {
					t.Fatalf("unexpected session: %s", session)
				}
				return nil
			},
			Reload: func() ([]Session, error) {
				return sessions, nil
			},
		},
	}
	model.resize()
	model.visible = filteredTreeRows(sessions, "", model.windowSort)

	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !deleted {
		t.Fatal("expected delete session to be called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected mode to return to browse, got %v", out.mode)
	}
}

func TestHandlePromptKeyRenameWindow(t *testing.T) {
	called := false
	model := pickerModel{
		windowSort: DefaultSortOptions().Window,
		viewport:   viewport.New(),
		width:      80,
		height:     20,
		queryInput: textinput.New(),
		promptInput: func() textinput.Model {
			in := textinput.New()
			in.SetValue("new-name")
			return in
		}(),
		mode:    modeRenameWindow,
		pending: Target{SessionName: "demo", WindowIndex: ptr(1)},
		actions: Actions{
			RenameWindow: func(session string, windowIndex int, name string) error {
				called = true
				if session != "demo" || windowIndex != 1 || name != "new-name" {
					t.Fatalf("unexpected args: %s %d %s", session, windowIndex, name)
				}
				return nil
			},
			Reload: func() ([]Session, error) { return nil, nil },
		},
	}
	model.resize()

	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !called {
		t.Fatal("expected RenameWindow to be called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected mode to return to browse, got %v", out.mode)
	}
}

func TestHandlePromptKeyNewSession(t *testing.T) {
	called := false
	model := pickerModel{
		windowSort: DefaultSortOptions().Window,
		viewport:   viewport.New(),
		width:      80,
		height:     20,
		queryInput: textinput.New(),
		promptInput: func() textinput.Model {
			in := textinput.New()
			in.SetValue("work")
			return in
		}(),
		mode:    modeNewSession,
		pending: Target{},
		actions: Actions{
			NewSession: func(name string) error {
				called = true
				if name != "work" {
					t.Fatalf("unexpected name: %s", name)
				}
				return nil
			},
			Reload: func() ([]Session, error) { return nil, nil },
		},
	}
	model.resize()

	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := next.(pickerModel)

	if !called {
		t.Fatal("expected NewSession to be called")
	}

	if out.mode != modeBrowse {
		t.Fatalf("expected mode to return to browse, got %v", out.mode)
	}
}

func TestHandlePromptKeyEscCancelsPrompt(t *testing.T) {
	model := pickerModel{
		viewport:    viewport.New(),
		width:       80,
		height:      20,
		queryInput:  textinput.New(),
		promptInput: textinput.New(),
		mode:        modeRenameSession,
	}
	model.resize()

	next, _ := model.handlePromptKey(tea.KeyPressMsg{Code: tea.KeyEsc})

	out := next.(pickerModel)
	if out.mode != modeBrowse {
		t.Fatalf("expected mode to return to browse, got %v", out.mode)
	}
}

func ptr[T any](v T) *T {
	return &v
}
