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
	}, themeDark, []string{"control"})

	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}
	if view := model.View().Content; !strings.Contains(view, "bravo  ←") {
		t.Fatalf("view does not mark current session: %q", view)
	}
}

func TestQuickPickerCanStartOnNextSession(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: true, Working: false},
		{Name: "bravo", Restored: true, Current: false, Working: false},
	}, themeDark, []string{"command"})
	model = model.moved(1)

	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}
}

func TestQuickPickerNavigationWraps(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: false, Current: false, Working: false},
		{Name: "bravo", Restored: false, Current: false, Working: false},
	}, themeDark, []string{"control"})
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	if model.cursor != 1 {
		t.Fatalf("up cursor = %d, want 1", model.cursor)
	}

	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if model.cursor != 0 {
		t.Fatalf("down cursor = %d, want 0", model.cursor)
	}
}

func TestQuickPickerUsesConfiguredNavigationModifiers(t *testing.T) {
	t.Parallel()

	sessions := []QuickSession{
		{Name: "alpha", Restored: true, Current: true, Working: false},
		{Name: "bravo", Restored: true, Current: false, Working: false},
	}
	commandOnly := newQuickPickerModel(sessions, themeDark, []string{"command"})
	commandOnly = updateQuick(t, commandOnly, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if commandOnly.cursor != 0 {
		t.Fatalf("disabled control modifier moved cursor to %d", commandOnly.cursor)
	}
	commandOnly = updateQuick(t, commandOnly, tea.KeyPressMsg{Code: 'j', Mod: tea.ModSuper})
	if commandOnly.cursor != 1 {
		t.Fatalf("command+j cursor = %d, want 1", commandOnly.cursor)
	}

	controlOnly := newQuickPickerModel(sessions, themeDark, []string{"control"})
	controlOnly = updateQuick(t, controlOnly, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if controlOnly.cursor != 1 {
		t.Fatalf("control+k cursor = %d, want wrapped cursor 1", controlOnly.cursor)
	}
	controlOnly = updateQuick(t, controlOnly, tea.KeyPressMsg{Code: 'k', Mod: tea.ModSuper})
	if controlOnly.cursor != 1 {
		t.Fatalf("disabled command modifier moved cursor to %d", controlOnly.cursor)
	}
}

func TestQuickPickerSupportsBothNavigationModifiers(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: true, Working: false},
		{Name: "bravo", Restored: true, Current: false, Working: false},
	}, themeDark, []string{"command", "control"})

	model = updateQuick(t, model, tea.KeyPressMsg{Code: 'j', Mod: tea.ModSuper})
	model = updateQuick(t, model, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after command+j and control+k", model.cursor)
	}
	if hints := model.helpHints(); !strings.Contains(hints, "⌘j/⌘k") ||
		!strings.Contains(hints, "^j/^k") {
		t.Fatalf("help hints do not show both modifiers: %q", hints)
	}
}

func TestQuickPickerSupportsTmuxCommandUserKeys(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: true, Working: false},
		{Name: "bravo", Restored: true, Current: false, Working: false},
	}, themeDark, []string{"command"})

	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyF11})
	if model.cursor != 1 {
		t.Fatalf("tmux User2 cursor = %d, want 1", model.cursor)
	}

	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyF12})
	if model.cursor != 0 {
		t.Fatalf("tmux User3 cursor = %d, want 0", model.cursor)
	}
}

func TestQuickPickerSelectsSession(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: false, Current: false, Working: false},
	}, themeDark, []string{"control"})
	model = updateQuick(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if model.selected != "alpha" {
		t.Fatalf("selected = %q, want alpha", model.selected)
	}
}

func TestQuickPickerFitsNarrowPopup(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "a-very-long-session-name", Restored: false, Current: false, Working: false},
	}, themeDark, []string{"control"})
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
	}, themeDark, []string{"control"})
	if !strings.Contains(model.View().Content, workingSpinnerFrames[0]) {
		t.Fatal("working session should render the first spinner frame")
	}

	model = updateQuick(t, model, spinnerTickMsg{})
	if !strings.Contains(model.View().Content, workingSpinnerFrames[1]) {
		t.Fatal("working session should advance the spinner frame")
	}
}

func TestQuickPickerSelectedWorkingSessionKeepsBackgroundAcrossRow(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "working", Restored: true, Current: false, Working: true},
	}, themeDark, []string{"control"})
	width := 28
	contentWidth := width - frameChromeWidth
	state := workingSpinnerFrames[0]
	want := model.theme.selBar.Render("  ") +
		model.theme.statusStyleOn(StatusWorking, true).Render(state) +
		model.theme.selBar.Width(contentWidth-3).Render(" working")

	if got := model.renderSession(0, width); got != want {
		t.Fatalf("selected working row styling:\n got %q\nwant %q", got, want)
	}
}

func TestQuickPickerLoadsWorkingStatusesAfterInitialization(t *testing.T) {
	t.Parallel()

	loadCalls := 0
	loader := func() map[string]bool {
		loadCalls++

		return map[string]bool{"bravo": true}
	}
	model := newQuickPickerModel([]QuickSession{
		{Name: "alpha", Restored: true, Current: false, Working: false},
		{Name: "bravo", Restored: true, Current: true, Working: false},
	}, themeDark, []string{"control"}, loader)
	if loadCalls != 0 {
		t.Fatal("status loader ran before the picker initialized")
	}
	if model.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", model.cursor)
	}

	msg := loadQuickStatuses(loader)()
	model = updateQuick(t, model, msg)
	if loadCalls != 1 || !model.visible[1].Working {
		t.Fatalf("working status was not applied: calls=%d visible=%+v", loadCalls, model.visible)
	}
	if model.cursor != 1 {
		t.Fatalf("async status update moved cursor to %d", model.cursor)
	}
}

func TestQuickPickerFiltersAndSelectsSessions(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "project", Restored: true, Current: true, Working: false},
		{Name: "jumpbox", Restored: true, Current: false, Working: false},
		{Name: "kubernetes", Restored: true, Current: false, Working: false},
	}, themeDark, []string{"control"})

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
	}, themeDark, []string{"control"})
	model = updateQuick(t, model, keyRune('z'))

	if len(model.visible) != 0 {
		t.Fatalf("visible sessions = %+v, want none", model.visible)
	}
	if !strings.Contains(model.View().Content, "No sessions match query") {
		t.Fatal("empty search result message is missing")
	}
}

func TestQuickPickerSearchKeepsLiveSessionsAboveSleeping(t *testing.T) {
	t.Parallel()

	model := newQuickPickerModel([]QuickSession{
		{Name: "x-prod-old", Restored: true, Current: false, Working: false},
		{Name: "prod", Restored: false, Current: false, Working: false},
	}, themeDark, []string{"control"})
	for _, key := range "prod" {
		model = updateQuick(t, model, keyRune(key))
	}

	if len(model.visible) != 2 || model.visible[0].Name != "x-prod-old" ||
		model.visible[1].Name != "prod" {
		t.Fatalf("search mixed live and sleeping groups: %+v", model.visible)
	}
}
