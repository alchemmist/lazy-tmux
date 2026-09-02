//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func updateQuick(t *testing.T, model quickPickerModel, msg tea.Msg) quickPickerModel {
	t.Helper()

	next, _ := model.Update(msg)
	result, ok := next.(quickPickerModel)
	if !ok {
		t.Fatalf("unexpected model type %T", next)
	}

	return result
}

func TestQuickPickerStartsOnCurrentSession(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true},
		{Name: "bravo", Restored: true, Current: true},
		{Name: "charlie"},
	}, themeDark)

	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}
	if view := model.View().Content; !strings.Contains(view, "bravo  current") {
		t.Fatalf("view does not mark current session: %q", view)
	}
}

func TestQuickPickerNavigationWraps(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{{Name: "alpha"}, {Name: "bravo"}}, themeDark)
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	if model.cursor != 1 {
		t.Fatalf("up cursor = %d, want 1", model.cursor)
	}

	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if model.cursor != 0 {
		t.Fatalf("down cursor = %d, want 0", model.cursor)
	}
}

func TestQuickPickerSelectsSession(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{{Name: "alpha"}}, themeDark)
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if model.selected != "alpha" {
		t.Fatalf("selected = %q, want alpha", model.selected)
	}
}

func TestQuickPickerFitsNarrowPopup(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{{Name: "a-very-long-session-name"}}, themeDark)
	model.width = 20
	model.height = 10

	for line := range strings.SplitSeq(model.View().Content, "\n") {
		if displayWidth(line) > 20 {
			t.Fatalf("line width = %d, want <= 20: %q", displayWidth(line), line)
		}
	}
}
