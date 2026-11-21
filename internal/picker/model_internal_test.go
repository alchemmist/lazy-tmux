//go:build !lazy_fzf

package picker

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errBoom = errors.New("boom")

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyCode(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
func keyCtrl(r rune) tea.KeyPressMsg    { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }
func keyAlt(r rune) tea.KeyPressMsg     { return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt} }

func feed(t *testing.T, m pickerModel, msg tea.Msg) pickerModel {
	t.Helper()

	next, _ := m.Update(msg)

	res, ok := next.(pickerModel)
	if !ok {
		t.Fatalf("Update returned unexpected model type %T", next)
	}

	return res
}

func makeSession(name string, restored bool, windowNames ...string) Session {
	windows := make([]snapshot.Window, 0, len(windowNames))
	for i, wn := range windowNames {
		windows = append(windows, snapshot.Window{
			Index: i + 1,
			Name:  wn,
			Panes: []snapshot.Pane{{Index: 0, CurrentCmd: "zsh"}},
		})
	}

	return Session{
		Record: snapshot.Record{
			SessionName: name,
			CapturedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Windows:     len(windowNames),
			Panes:       len(windowNames),
		},
		Windows:  windows,
		Restored: restored,
	}
}

func newTestModel(t *testing.T, sessions ...Session) pickerModel {
	t.Helper()
	m := newPickerModel(sessions, DefaultSortOptions().Window, Actions{})

	return feed(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
}

func workingSession(name string) Session {
	sess := makeSession(name, true, "one")
	sess.Statuses = map[int]WindowStatus{1: StatusWorking}

	return sess
}

func largeWorkingPickerModel() pickerModel {
	sessions := make([]Session, 100)
	for sessionIndex := range sessions {
		windowNames := make([]string, 10)
		for windowIndex := range windowNames {
			windowNames[windowIndex] = fmt.Sprintf("window-%03d-%02d", sessionIndex, windowIndex)
		}

		sessions[sessionIndex] = makeSession(
			fmt.Sprintf("session-%03d", sessionIndex),
			true,
			windowNames...,
		)
	}
	sessions[0].Statuses = map[int]WindowStatus{1: StatusWorking}

	m := newPickerModel(sessions, DefaultSortOptions().Window, Actions{})
	m.queryInput.SetValue("session")
	m.applyFilter()
	m.width = 120
	m.height = 30
	m.resize()
	m.renderViewport()

	return m
}

//nolint:paralleltest // AllocsPerRun cannot execute in a parallel test
func TestSpinnerTickAllocationBudget(t *testing.T) {
	m := largeWorkingPickerModel()
	allocations := testing.AllocsPerRun(10, func() {
		m = spinnerTickModel(m)
	})

	if allocations > 300 {
		t.Fatalf("spinner tick allocated %.0f objects, want at most 300", allocations)
	}
}

func BenchmarkSpinnerTickLargePicker(b *testing.B) {
	m := largeWorkingPickerModel()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		m = spinnerTickModel(m)
	}
}

func spinnerTickModel(m pickerModel) pickerModel {
	next, _ := m.handleSpinnerTick()
	updated, ok := next.(pickerModel)
	if !ok {
		panic(fmt.Sprintf("unexpected spinner model type %T", next))
	}

	return updated
}

func TestHasWorkingWindow(t *testing.T) {
	t.Parallel()

	if !newTestModel(t, workingSession("w")).hasWorkingWindow() {
		t.Fatal("a working window must be detected")
	}

	idle := makeSession("i", true, "one")
	idle.Statuses = map[int]WindowStatus{1: StatusIdle}

	if newTestModel(t, idle).hasWorkingWindow() {
		t.Fatal("no working window must be reported")
	}
}

func TestApplyThemeCommand(t *testing.T) {
	t.Parallel()

	m := newPickerModelWithTheme(nil, nil, Actions{}, "dark")
	m.applyThemeCommand("light")

	if m.themeName != "light" {
		t.Fatalf("theme was not switched: %q", m.themeName)
	}

	if m.theme.selBg != lightSelBg {
		t.Fatalf("light selection background = %q, want %q", m.theme.selBg, lightSelBg)
	}
}

func TestSpinnerTickAdvancesFrameWhileWorking(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, workingSession("w"))
	before := m.spinnerFrame

	next, cmd := m.handleSpinnerTick()
	updated, _ := next.(pickerModel)

	if updated.spinnerFrame != before+1 {
		t.Fatalf("frame should advance, got %d want %d", updated.spinnerFrame, before+1)
	}

	if cmd == nil {
		t.Fatal("a working spinner tick must reschedule itself")
	}
}

func TestSpinnerTickStopsWhenIdle(t *testing.T) {
	t.Parallel()

	idle := makeSession("i", true, "one")
	idle.Statuses = map[int]WindowStatus{1: StatusIdle}

	m := newTestModel(t, idle)
	m.spinnerOn = true

	next, cmd := m.handleSpinnerTick()
	updated, _ := next.(pickerModel)

	if updated.spinnerOn {
		t.Fatal("spinner must switch off when no window is working")
	}

	if cmd != nil {
		t.Fatal("a stopped spinner must not reschedule")
	}
}

func TestStatusTickResumesSpinnerOnWork(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, workingSession("w"))
	m.spinnerOn = false

	next, cmd := m.handleStatusTick()
	updated, _ := next.(pickerModel)

	if !updated.spinnerOn {
		t.Fatal("a status tick must resume the spinner when a working window exists")
	}

	if cmd == nil {
		t.Fatal("status tick must schedule follow-up commands")
	}
}

func TestLoadingModelRendersBeforeSessionsArrive(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.loading = true

	if view := m.View().Content; !strings.Contains(view, "Loading sessions…") {
		t.Fatalf("loading view missing from first frame:\n%s", view)
	}
}

func TestOutlineRendersBeforeDetailsAndStatus(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, makeSession("alpha", true, "one"))
	m.sessions[0].Windows = nil
	m.loading = false
	m.applyFilter()
	m.renderViewport()
	view := m.View().Content
	if !strings.Contains(view, "alpha") || strings.Contains(view, "Loading sessions…") {
		t.Fatalf("outline was not visible immediately: %s", view)
	}
	m.reloadGeneration = 1
	m = feed(t, m, sessionsLoadedMsg{
		sessions:   []Session{makeSession("alpha", true, "one")},
		generation: 1,
	})
	m.statusGeneration = 1
	m = feed(t, m, statusesLoadedMsg{
		generation: 1,
		statuses:   map[string]map[int]WindowStatus{"alpha": {1: StatusWorking}},
	})
	if m.sessions[0].Statuses[1] != StatusWorking {
		t.Fatalf("status was not applied lazily: %+v", m.sessions[0].Statuses)
	}
	if view := m.View().Content; !strings.Contains(view, "one") {
		t.Fatalf("window missing after background load: %s", view)
	}
}

func TestSessionsLoadedMessagePopulatesPicker(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.loading = true
	m.reloadPending = true
	m = feed(t, m, sessionsLoadedMsg{
		sessions: []Session{makeSession("alpha", false, "one")},
	})

	if m.loading || m.reloadPending {
		t.Fatalf("load flags were not cleared: loading=%v pending=%v", m.loading, m.reloadPending)
	}
	if len(m.sessions) != 1 || m.sessions[0].Record.SessionName != "alpha" {
		t.Fatalf("loaded sessions were not applied: %+v", m.sessions)
	}
	if len(m.visible) == 0 {
		t.Fatal("loaded sessions were not rendered")
	}
}

func TestSessionsLoadedMessageRendersError(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.loading = true
	m = feed(t, m, sessionsLoadedMsg{err: errBoom})

	if m.loading {
		t.Fatal("loading flag must clear after an error")
	}
	if !strings.Contains(m.View().Content, errBoom.Error()) {
		t.Fatalf("loader error is missing from view: %s", m.View().Content)
	}
}

func TestStaleReloadDoesNotOverwriteMutationResult(t *testing.T) {
	t.Parallel()

	current := []Session{makeSession("remaining", false, "one")}
	m := newPickerModel([]Session{makeSession("deleted", false, "old")}, nil, Actions{
		Reload: func() ([]Session, error) {
			return current, nil
		},
	})
	m.reloadGeneration = 1
	m.reloadPending = true
	m.applyActionResult(errBoom)
	mutationGeneration := m.reloadGeneration

	next, _ := m.handleStatusTick()
	m, _ = next.(pickerModel)
	if m.reloadGeneration != mutationGeneration {
		t.Fatal("status tick scheduled another reload while an older reload was in flight")
	}
	if !m.reloadPending {
		t.Fatal("mutation reload cleared the in-flight reload marker")
	}

	m = feed(t, m, sessionsLoadedMsg{
		sessions:   []Session{makeSession("deleted", false, "old")},
		generation: 1,
	})

	if len(m.sessions) != 1 || m.sessions[0].Record.SessionName != "remaining" {
		t.Fatalf("stale reload overwrote mutation result: %+v", m.sessions)
	}
	if m.statusMsg != errBoom.Error() {
		t.Fatalf("stale reload changed action status: %q", m.statusMsg)
	}
	if m.reloadPending {
		t.Fatal("completed stale reload did not clear the in-flight marker")
	}
}

func TestMutationReloadEndsInitialLoading(t *testing.T) {
	t.Parallel()

	current := []Session{makeSession("created", false, "one")}
	m := newPickerModel(nil, nil, Actions{
		Reload: func() ([]Session, error) {
			return current, nil
		},
	})
	m.loading = true
	m.reloadPending = true
	m.reloadGeneration = 1
	m.applyActionResult(nil)

	if m.loading {
		t.Fatal("mutation reload left the picker in its initial loading state")
	}
	if !m.reloadPending {
		t.Fatal("mutation reload lost track of the initial load still in flight")
	}
	if len(m.sessions) != 1 || m.sessions[0].Record.SessionName != "created" {
		t.Fatalf("mutation reload did not apply current sessions: %+v", m.sessions)
	}

	m = feed(t, m, sessionsLoadedMsg{
		sessions:   []Session{},
		generation: 1,
	})
	if m.loading || m.reloadPending {
		t.Fatalf(
			"stale initial load changed lifecycle flags: loading=%v pending=%v",
			m.loading,
			m.reloadPending,
		)
	}
	if len(m.sessions) != 1 || m.sessions[0].Record.SessionName != "created" {
		t.Fatalf("stale initial load overwrote mutation sessions: %+v", m.sessions)
	}
}

func TestMutationReloadWaitsForBackgroundReload(t *testing.T) {
	t.Parallel()

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseFirst)
		}
	}()

	calls := 0
	m := newPickerModel(nil, nil, Actions{
		Reload: func() ([]Session, error) {
			calls++
			if calls == 1 {
				close(firstStarted)
				<-releaseFirst
			} else {
				close(secondStarted)
			}

			return nil, nil
		},
	})

	backgroundDone := make(chan struct{})
	go func() {
		_ = loadSessions(m.actions.Reload, 1)()
		close(backgroundDone)
	}()
	<-firstStarted

	mutationDone := make(chan struct{})
	go func() {
		m.reload()
		close(mutationDone)
	}()

	select {
	case <-secondStarted:
		t.Fatal("mutation loader started while background loader was still running")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	released = true
	<-backgroundDone
	<-secondStarted
	<-mutationDone
}

func TestSuccessfulRefreshPreservesActionStatus(t *testing.T) {
	t.Parallel()

	m := newPickerModel(nil, nil, Actions{})
	m.reloadGeneration = 1
	m.setStatus("delete failed")
	m = feed(t, m, sessionsLoadedMsg{
		sessions:   []Session{makeSession("alpha", false, "one")},
		generation: 1,
	})

	if m.statusMsg != "delete failed" {
		t.Fatalf("successful refresh cleared action status: %q", m.statusMsg)
	}
	if len(m.sessions) != 1 || m.sessions[0].Record.SessionName != "alpha" {
		t.Fatalf("successful refresh did not apply sessions: %+v", m.sessions)
	}
}

func TestSuccessfulRefreshClearsLoaderStatus(t *testing.T) {
	t.Parallel()

	m := newPickerModel(nil, nil, Actions{})
	m.reloadGeneration = 1
	m.setLoaderStatus("load failed")
	m = feed(t, m, sessionsLoadedMsg{
		sessions:   []Session{makeSession("alpha", false, "one")},
		generation: 1,
	})

	if m.statusMsg != "" || m.statusFromLoader {
		t.Fatalf("successful refresh kept loader status: %q", m.statusMsg)
	}
}

func TestStatusTickSchedulesReloadWithoutBlocking(t *testing.T) {
	t.Parallel()

	called := false
	m := newPickerModel(nil, nil, Actions{
		Reload: func() ([]Session, error) {
			called = true

			return nil, nil
		},
	})

	next, cmd := m.handleStatusTick()
	updated, _ := next.(pickerModel)
	if called {
		t.Fatal("status tick called the session loader synchronously")
	}
	if !updated.reloadPending {
		t.Fatal("status tick did not mark the asynchronous reload as pending")
	}
	if cmd == nil {
		t.Fatal("status tick did not schedule reload commands")
	}
}

func TestEscapeCancelsWhileSessionsLoad(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.loading = true
	m = feed(t, m, keyCode(tea.KeyEscape))

	if !m.cancelled {
		t.Fatal("escape must cancel the picker while sessions load")
	}
}

func TestModelNavigationAndSelect(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("alpha", false, "one", "two"),
		makeSession("beta", true, "three"),
	)

	if m.cursor != 1 || !m.visible[m.cursor].selectable {
		t.Fatalf("cursor should start on first selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('j'))
	if m.cursor != 2 {
		t.Fatalf("ctrl+j should move to next selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('j'))
	if m.cursor != 4 {
		t.Fatalf("ctrl+j should skip header to beta window, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('k'))
	if m.cursor != 2 {
		t.Fatalf("ctrl+k should move to prev selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if m.selected.SessionName != "alpha" || m.selected.WindowIndex == nil ||
		*m.selected.WindowIndex != 2 {
		t.Fatalf("enter should select alpha window 2, got %+v", m.selected)
	}
}

func TestModelJumpToFirstVisibleWindowByIndex(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("alpha", false, "one", "needle"),
		makeSession("beta", true, "needle", "three"),
	)

	for _, r := range "needle" {
		m = feed(t, m, keyRune(r))
	}
	m = feed(t, m, keyAlt('1'))

	if m.visible[m.cursor].target.SessionName != "beta" ||
		m.visible[m.cursor].target.WindowIndex == nil ||
		*m.visible[m.cursor].target.WindowIndex != 1 {
		t.Fatalf(
			"alt+1 should jump to beta window 1, got cursor=%d row=%+v",
			m.cursor,
			m.visible[m.cursor],
		)
	}
	if m.selected.SessionName != "beta" || m.selected.WindowIndex == nil ||
		*m.selected.WindowIndex != 1 {
		t.Fatalf("alt+1 should select beta window 1, got %+v", m.selected)
	}
}

func TestModelJumpToWindowSupportsAltModifier(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one", "two"))
	m = feed(t, m, keyAlt('2'))

	if m.visible[m.cursor].target.WindowIndex == nil ||
		*m.visible[m.cursor].target.WindowIndex != 2 {
		t.Fatalf(
			"alt+2 should jump to window 2, got cursor=%d row=%+v",
			m.cursor,
			m.visible[m.cursor],
		)
	}
}

func TestModelJumpToWindowSupportsAltKeypadDigit(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one", "two"))
	m.cursor = 2
	m = feed(t, m, tea.KeyPressMsg{Code: tea.KeyKp1, Mod: tea.ModAlt})

	if m.visible[m.cursor].target.WindowIndex == nil ||
		*m.visible[m.cursor].target.WindowIndex != 1 {
		t.Fatalf(
			"alt+keypad 1 should jump to window 1, got cursor=%d row=%+v",
			m.cursor,
			m.visible[m.cursor],
		)
	}
}

func TestModelJumpToWindowSupportsTerminalDigitFallback(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one", "two"))
	m.cursor = 2
	m = feed(t, m, tea.KeyPressMsg{BaseCode: '1', Mod: tea.ModAlt})

	if m.visible[m.cursor].target.WindowIndex == nil ||
		*m.visible[m.cursor].target.WindowIndex != 1 {
		t.Fatalf(
			"terminal digit fallback should jump to window 1, got cursor=%d row=%+v",
			m.cursor,
			m.visible[m.cursor],
		)
	}
}

func TestModelJumpToWindowIgnoresNonAltModifiers(t *testing.T) {
	t.Parallel()

	for _, mod := range []tea.KeyMod{tea.ModCtrl, tea.ModMeta, tea.ModSuper} {
		if _, ok := windowJumpIndex(tea.KeyPressMsg{Code: '1', Mod: mod}); ok {
			t.Fatalf("modifier %v should not trigger a window jump", mod)
		}
	}
}

func TestModelJumpToMissingWindowDoesNothing(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one", "two"))
	m.cursor = 2
	before := m.cursor
	m = feed(t, m, keyAlt('9'))

	if m.cursor != before {
		t.Fatalf("missing window index should keep cursor at %d, got %d", before, m.cursor)
	}
}

func TestModelFilter(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("alpha", false, "one"),
		makeSession("beta", false, "two"),
	)

	m = feed(t, m, keyRune('b'))

	if len(m.visible) == 0 {
		t.Fatal("expected some visible rows after filtering")
	}

	for _, row := range m.visible {
		if row.target.SessionName != "beta" {
			t.Fatalf("filter 'b' should only show beta, got %q", row.target.SessionName)
		}
	}
}

func TestModelActionModePreservesFilter(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("alpha", false, "one"),
		makeSession("beta", false, "two"),
	)

	m = feed(t, m, keyRune('b'))
	wantRows := append([]pickerRow(nil), m.visible...)
	m = feed(t, m, keyAlt('d'))

	if got := m.queryInput.Value(); got != "b" {
		t.Fatalf("alt+d should preserve query %q, got %q", "b", got)
	}
	if !sameVisibleTargets(m.visible, wantRows) {
		t.Fatalf("alt+d should preserve filtered rows, got %+v, want %+v", m.visible, wantRows)
	}

	m = feed(t, m, keyCode(tea.KeyEscape))
	if got := m.queryInput.Value(); got != "b" {
		t.Fatalf("exiting delete mode should restore query %q, got %q", "b", got)
	}
}

func sameVisibleTargets(got, want []pickerRow) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i].target.SessionName != want[i].target.SessionName ||
			!sameWindowIndex(got[i].target.WindowIndex, want[i].target.WindowIndex) {
			return false
		}
	}

	return true
}

func sameWindowIndex(got, want *int) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

func TestModelEscCancels(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"))

	m = feed(t, m, keyCode(tea.KeyEscape))
	if !m.cancelled {
		t.Fatal("esc should mark the model cancelled")
	}
}

func TestModelNewFilteringBindsToSession(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"), makeSession("beta", false, "two"))

	m = feed(t, m, keyAlt('n'))

	m = feed(t, m, keyRune('b'))

	for _, row := range m.visible {
		if row.synthetic {
			t.Fatal("synthetic new-session row must be hidden while filtering")
		}
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if m.mode != modeNewWindow {
		t.Fatalf("filtered ↵ should bind to new-window, got %d", m.mode)
	}
}

func TestModelWakeFiltersToSleeping(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("sleepy", false, "one"),
		makeSession("live", true, "two"),
	)

	m = feed(t, m, keyAlt('w'))

	for _, row := range m.visible {
		if row.target.SessionName == "live" {
			t.Fatal("wake mode must hide live sessions")
		}
	}
}

func TestModelSleepFiltersToLive(t *testing.T) {
	t.Parallel()

	m := newTestModel(t,
		makeSession("sleepy", false, "one"),
		makeSession("live", true, "two"),
	)

	m = feed(t, m, keyAlt('s'))

	for _, row := range m.visible {
		if row.target.SessionName == "sleepy" {
			t.Fatal("sleep mode must hide sleeping sessions")
		}
	}
}

func TestModelEscExitsActionMode(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"))

	m = feed(t, m, keyAlt('d'))
	if m.action != actionDelete {
		t.Fatalf("alt+d should enter delete mode, got %d", m.action)
	}

	m = feed(t, m, keyCode(tea.KeyEscape))

	if m.action != actionBrowse {
		t.Fatalf("esc should drop back to browse, got %d", m.action)
	}

	if m.cancelled {
		t.Fatal("esc in a mode must cancel the mode, not the picker")
	}
}

func TestModelPaletteOpensOnSlash(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"))

	m = feed(t, m, keyRune('/'))
	if !m.palette {
		t.Fatal("typing / should open the command palette")
	}

	for _, r := range "del" {
		m = feed(t, m, keyRune(r))
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if m.action != actionDelete {
		t.Fatalf("/del ↵ should enter delete mode, got %d", m.action)
	}

	if m.palette {
		t.Fatal("palette should close once a command runs")
	}
}

func TestModelViewRendersWithoutPanic(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"))

	if got := m.View(); got.AltScreen != true {
		t.Fatal("expected alt screen view")
	}

	m = feed(t, m, keyRune('z'))
	m = feed(t, m, keyRune('z'))
	m = feed(t, m, keyRune('z'))
	_ = m.View()
}

func TestFilteredTreeRowsAndTable(t *testing.T) {
	t.Parallel()

	sessions := []Session{makeSession("alpha", true, "edit", "shell")}

	rows := filteredTreeRows(sessions, "", DefaultSortOptions().Window, 0)
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 window rows, got %d", len(rows))
	}

	if rows[0].selectable {
		t.Fatal("session header row should not be selectable")
	}

	if !rows[1].selectable || rows[1].target.WindowIndex == nil {
		t.Fatal("window rows should be selectable with a window index")
	}

	layout := buildPickerTableLayout(120)

	header := layout.header()
	if header == "" {
		t.Fatal("expected a non-empty table header")
	}

	row := layout.row(rows[0])
	if row == "" {
		t.Fatal("expected a non-empty rendered row")
	}

	narrow := buildPickerTableLayout(12)
	if narrow.row(rows[1]) == "" {
		t.Fatal("expected narrow row to render")
	}
}

func TestFilteredTreeRowsRanksExactWindowFirst(t *testing.T) {
	t.Parallel()

	sessions := []Session{
		makeSession("arcadia-turbo", true, "antex", "code"),
		makeSession("arcadia", true, "pilot", "code"),
		makeSession("monori", true, "CI"),
	}

	rows := filteredTreeRows(sessions, "ci", DefaultSortOptions().Window, 0)
	if len(rows) < 2 {
		t.Fatalf("expected exact match rows, got %d", len(rows))
	}
	if rows[0].target.SessionName != "monori" {
		t.Fatalf("exact window session should rank first, got %q", rows[0].target.SessionName)
	}
	if rows[1].windowName != "CI" {
		t.Fatalf("exact window should rank first, got %q", rows[1].windowName)
	}
}

func TestFilteredTreeRowsRanksMatchingWindowWithinSession(t *testing.T) {
	t.Parallel()

	sessions := []Session{makeSession("project", true, "shell", "antex", "code")}
	rows := filteredTreeRows(sessions, "antex", DefaultSortOptions().Window, 0)

	if len(rows) < 2 || rows[1].windowName != "antex" {
		t.Fatalf("expected exact matching window first, got %+v", rows)
	}
}

func TestPickerWithoutSelection(t *testing.T) {
	t.Parallel()
	if _, err := (pickerModel{}).chosenTarget(); !errors.Is(err, errNoSessionSelected) {
		t.Fatalf("empty selection: %v", err)
	}
}

func TestModelHelpToggle(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one", "two"))

	m = feed(t, m, keyRune('?'))
	if !m.helpOpen {
		t.Fatal("'?' should open the help panel")
	}

	view := m.View().Content
	if !strings.Contains(view, "Navigate") || !strings.Contains(view, "sleep a live session") {
		t.Fatalf("help panel should list keybindings, got:\n%s", view)
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if m.helpOpen {
		t.Fatal("any key should close the help panel")
	}

	if m.selected.SessionName != "" {
		t.Fatal("the key that closed help must not also select a row")
	}
}

func TestModelHelpNotTriggeredWhileTyping(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, makeSession("alpha", false, "one"))

	m = feed(t, m, keyRune('a'))
	m = feed(t, m, keyRune('?'))

	if m.helpOpen {
		t.Fatal("'?' must not open help while typing a query")
	}
}

func TestPiWorkingUsesSharedSpinnerAndStopsWhenAwaitingInput(t *testing.T) {
	t.Parallel()
	session := workingSession("pi-project")
	session.Windows[0].Panes = []snapshot.Pane{{CurrentCmd: "node", RestoreCmd: "pi"}}
	model := newTestModel(t, session)
	if !model.hasWorkingWindow() {
		t.Fatal("Pi working state did not enable animation")
	}
	before := model.View().Content
	model = spinnerTickModel(model)
	if after := model.View().Content; before == after {
		t.Fatal("Pi working indicator did not animate")
	}
	session.Statuses[1] = StatusAwaitingInput
	idle := newTestModel(t, session)
	if idle.hasWorkingWindow() {
		t.Fatal("Pi waiting for input still animated")
	}
}
