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
		{Name: "alpha", Restored: true, Current: false, Working: false},
		{Name: "bravo", Restored: true, Current: true, Working: false},
		{Name: "charlie", Restored: false, Current: false, Working: false},
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

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: false, Current: false, Working: false},
		{Name: "bravo", Restored: false, Current: false, Working: false},
	}, themeDark)
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

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: false, Current: false, Working: false},
	}, themeDark)
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if model.selected != "alpha" {
		t.Fatalf("selected = %q, want alpha", model.selected)
	}
}

func TestQuickPickerFitsNarrowPopup(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "a-very-long-session-name", Restored: false, Current: false, Working: false},
	}, themeDark)
	model.width = 20
	model.height = 10

	for line := range strings.SplitSeq(model.View().Content, "\n") {
		if displayWidth(line) > 20 {
			t.Fatalf("line width = %d, want <= 20: %q", displayWidth(line), line)
		}
	}
}

func TestQuickPickerAnimatesWorkingSession(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: false, Working: true},
	}, themeDark)
	if !strings.Contains(model.View().Content, workingSpinnerFrames[0]) {
		t.Fatal("working session should render the first spinner frame")
	}

	model = updateQuick(t, model, spinnerTickMsg{})
	if !strings.Contains(model.View().Content, workingSpinnerFrames[1]) {
		t.Fatal("working session should advance the spinner frame")
	}
}

func TestQuickPickerFiltersAndSelectsSessions(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "project", Restored: true, Current: true, Working: false},
		{Name: "jumpbox", Restored: true, Current: false, Working: false},
		{Name: "kubernetes", Restored: true, Current: false, Working: false},
	}, themeDark)

	model = updateQuick(t, model, keyRune('j'))
	if model.queryInput.Value() != "j" {
		t.Fatalf("query = %q, want j", model.queryInput.Value())
	}
	if len(model.visible) != 2 {
		t.Fatalf("filtered sessions = %+v", model.visible)
	}

	model = updateQuick(t, model, keyRune('u'))
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.selected != "jumpbox" {
		t.Fatalf("selected = %q, want jumpbox", model.selected)
	}
}

func TestQuickPickerShowsEmptySearchResult(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: false, Working: false},
	}, themeDark)
	model = updateQuick(t, model, keyRune('z'))

	if len(model.visible) != 0 {
		t.Fatalf("visible sessions = %+v, want none", model.visible)
	}
	if !strings.Contains(model.View().Content, "No sessions match query") {
		t.Fatal("empty search result message is missing")
	}
}
