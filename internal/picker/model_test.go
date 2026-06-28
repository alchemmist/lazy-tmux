//go:build !lazy_fzf

package picker

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// keyRune builds a printable key press (routed into the query/prompt input).
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

// recordingActions captures which callbacks the model invoked. The Actions
// struct is the picker's real dependency boundary; production wires these to
// the app (which talks to real tmux). Driving them here exercises the model's
// dispatch logic without a terminal.
type recordingActions struct {
	deletedWindow  [2]int
	deletedWindowS string
	deletedSession string
	renamedWindow  string
	renamedSession string
	newSession     string
	newWindow      [2]string
	wokeUp         string
	slept          string
	failDelete     bool
}

func (r *recordingActions) toActions(sessions []Session) Actions {
	return Actions{
		DeleteWindow: func(s string, idx int) error {
			if r.failDelete {
				return errors.New("boom")
			}

			r.deletedWindowS = s
			r.deletedWindow = [2]int{idx, idx}

			return nil
		},
		DeleteSession: func(s string) error { r.deletedSession = s; return nil },
		RenameWindow:  func(s string, idx int, n string) error { r.renamedWindow = n; return nil },
		RenameSession: func(s, n string) error { r.renamedSession = n; return nil },
		NewSession:    func(n string) error { r.newSession = n; return nil },
		NewWindow:     func(s, n string) error { r.newWindow = [2]string{s, n}; return nil },
		Wakeup:        func(s string) error { r.wokeUp = s; return nil },
		Sleep:         func(s string) error { r.slept = s; return nil },
		Reload:        func() ([]Session, error) { return sessions, nil },
	}
}

func newTestModel(t *testing.T, rec *recordingActions, sessions ...Session) pickerModel {
	t.Helper()

	m := newPickerModel(sessions, DefaultSortOptions().Window, rec.toActions(sessions))
	m = feed(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})

	return m
}

func TestModelNavigationAndSelect(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec,
		makeSession("alpha", false, "one", "two"),
		makeSession("beta", true, "three"),
	)

	// Rows: [alpha hdr, alpha:1, alpha:2, beta hdr, beta:1]
	// Cursor starts on first selectable (alpha:1).
	if m.cursor != 1 || !m.visible[m.cursor].selectable {
		t.Fatalf("cursor should start on first selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('j')) // -> alpha:2
	if m.cursor != 2 {
		t.Fatalf("ctrl+j should move to next selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('j')) // -> beta:1
	if m.cursor != 4 {
		t.Fatalf("ctrl+j should skip header to beta window, got %d", m.cursor)
	}

	m = feed(t, m, keyCtrl('k')) // back to alpha:2
	if m.cursor != 2 {
		t.Fatalf("ctrl+k should move to prev selectable, got %d", m.cursor)
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if m.selected.SessionName != "alpha" || m.selected.WindowIndex == nil ||
		*m.selected.WindowIndex != 2 {
		t.Fatalf("enter should select alpha window 2, got %+v", m.selected)
	}
}

func TestModelFilter(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec,
		makeSession("alpha", false, "one"),
		makeSession("beta", false, "two"),
	)

	m = feed(t, m, keyRune('b')) // query "b" -> only beta matches

	if len(m.visible) == 0 {
		t.Fatal("expected some visible rows after filtering")
	}

	for _, row := range m.visible {
		if row.target.SessionName != "beta" {
			t.Fatalf("filter 'b' should only show beta, got %q", row.target.SessionName)
		}
	}
}

func TestModelEscCancels(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyCode(tea.KeyEscape))
	if !m.cancelled {
		t.Fatal("esc should mark the model cancelled")
	}
}

func TestModelDeleteWindow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one", "two"))

	// ^d enters delete mode and pre-marks the current window (one of two, so a
	// window — not the whole session — is removed); ↵ commits.
	m = feed(t, m, keyCtrl('d'))
	if m.action != actionDelete {
		t.Fatalf("ctrl+d should enter delete mode, got %d", m.action)
	}

	feed(t, m, keyCode(tea.KeyEnter))

	if rec.deletedWindowS != "alpha" || rec.deletedWindow[0] != 1 {
		t.Fatalf(
			"ctrl+d ↵ should delete current window, got %q idx=%d",
			rec.deletedWindowS,
			rec.deletedWindow[0],
		)
	}
}

func TestModelDeleteWindowErrorSetsStatus(t *testing.T) {
	rec := &recordingActions{failDelete: true}
	m := newTestModel(t, rec, makeSession("alpha", false, "one", "two"))

	m = feed(t, m, keyCtrl('d'))
	m = feed(t, m, keyCode(tea.KeyEnter))

	if m.statusMsg == "" {
		t.Fatal("a failing delete should set a status message")
	}
}

func TestModelDeleteSessionFlow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyAlt('d')) // delete mode, whole session pre-marked
	if m.action != actionDelete {
		t.Fatalf("alt+d should enter delete mode, got %d", m.action)
	}

	m = feed(t, m, keyCode(tea.KeyEnter))

	if rec.deletedSession != "alpha" {
		t.Fatalf("expected session alpha deleted, got %q", rec.deletedSession)
	}

	if m.action != actionBrowse || m.mode != modeBrowse {
		t.Fatalf("should return to browse, got action=%d mode=%d", m.action, m.mode)
	}
}

func TestModelDeleteMultiSelect(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one", "two", "three"))

	// /delete via the palette starts with nothing marked.
	m = feed(t, m, keyRune('/'))
	for _, r := range "delete" {
		m = feed(t, m, keyRune(r))
	}

	m = feed(t, m, keyCode(tea.KeyEnter)) // enter delete mode
	if m.action != actionDelete {
		t.Fatalf("/delete should enter delete mode, got %d", m.action)
	}

	// Mark the first two window rows (rows: [hdr, w1, w2, w3]).
	m.cursor = 1
	m = feed(t, m, keyRune(' '))
	m.cursor = 2
	m = feed(t, m, keyRune(' '))

	feed(t, m, keyCode(tea.KeyEnter))

	// Two of three windows marked -> per-window deletes, not a session delete.
	if rec.deletedSession != "" {
		t.Fatalf("partial selection must not delete the session, got %q", rec.deletedSession)
	}

	if rec.deletedWindowS != "alpha" {
		t.Fatalf("expected window deletes in alpha, got %q", rec.deletedWindowS)
	}
}

func TestModelRenameWindowFlow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyCtrl('r')) // rename mode, cursor on the window
	if m.action != actionRename {
		t.Fatalf("ctrl+r should enter rename mode, got %d", m.action)
	}

	m = feed(t, m, keyCode(tea.KeyEnter)) // open the prompt (preset to "one")
	if m.mode != modeRenameWindow {
		t.Fatalf("↵ should open the rename-window prompt, got %d", m.mode)
	}

	m = feed(t, m, keyRune('X'))
	feed(t, m, keyCode(tea.KeyEnter))

	if rec.renamedWindow != "oneX" {
		t.Fatalf("expected renamed window 'oneX', got %q", rec.renamedWindow)
	}
}

func TestModelRenameSessionFlow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyAlt('r')) // rename mode, cursor on the session header
	m = feed(t, m, keyCode(tea.KeyEnter))
	m = feed(t, m, keyRune('Z'))
	feed(t, m, keyCode(tea.KeyEnter))

	if rec.renamedSession != "alphaZ" {
		t.Fatalf("expected renamed session 'alphaZ', got %q", rec.renamedSession)
	}
}

func TestModelNewSessionFlow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyAlt('n')) // new mode, cursor on synthetic "+ new session"
	if m.action != actionNew {
		t.Fatalf("alt+n should enter new mode, got %d", m.action)
	}

	m = feed(t, m, keyCode(tea.KeyEnter)) // open new-session prompt
	if m.mode != modeNewSession {
		t.Fatalf("↵ on the synthetic row should open new-session, got %d", m.mode)
	}

	m = feed(t, m, keyRune('q'))
	m = feed(t, m, keyRune('a'))
	feed(t, m, keyCode(tea.KeyEnter))

	if rec.newSession != "qa" {
		t.Fatalf("expected new session 'qa', got %q", rec.newSession)
	}
}

func TestModelNewWindowFlow(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyCtrl('n')) // new mode, cursor on the alpha session header
	if m.action != actionNew {
		t.Fatalf("ctrl+n should enter new mode, got %d", m.action)
	}

	m = feed(t, m, keyCode(tea.KeyEnter)) // open new-window prompt
	if m.mode != modeNewWindow {
		t.Fatalf("↵ on a session row should open new-window, got %d", m.mode)
	}

	feed(t, m, keyCode(tea.KeyEnter)) // empty name allowed (auto-named)

	if rec.newWindow[0] != "alpha" {
		t.Fatalf("expected new window in alpha, got %q", rec.newWindow[0])
	}
}

func TestModelWakeFiltersToSleeping(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec,
		makeSession("sleepy", false, "one"), // not live -> wakeable
		makeSession("live", true, "two"),    // live -> not wakeable
	)

	m = feed(t, m, keyAlt('w')) // wake mode

	for _, row := range m.visible {
		if row.target.SessionName == "live" {
			t.Fatal("wake mode must hide live sessions")
		}
	}

	m = feed(t, m, keyCode(tea.KeyEnter))
	if rec.wokeUp != "sleepy" {
		t.Fatalf("↵ in wake mode should wake the sleeping session, got %q", rec.wokeUp)
	}

	if m.action != actionBrowse {
		t.Fatalf("acting should return to browse, got %d", m.action)
	}
}

func TestModelSleepFiltersToLive(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec,
		makeSession("sleepy", false, "one"),
		makeSession("live", true, "two"),
	)

	m = feed(t, m, keyAlt('s')) // sleep mode

	for _, row := range m.visible {
		if row.target.SessionName == "sleepy" {
			t.Fatal("sleep mode must hide sleeping sessions")
		}
	}

	feed(t, m, keyCode(tea.KeyEnter))
	if rec.slept != "live" {
		t.Fatalf("↵ in sleep mode should sleep the live session, got %q", rec.slept)
	}
}

func TestModelEscExitsActionMode(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyAlt('d')) // delete mode
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

	if rec.deletedSession != "" || rec.deletedWindowS != "" {
		t.Fatal("esc must not perform the action")
	}
}

func TestModelPaletteOpensOnSlash(t *testing.T) {
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	m = feed(t, m, keyRune('/'))
	if !m.palette {
		t.Fatal("typing / should open the command palette")
	}

	// Refine to "/del" and confirm: only delete matches, so ↵ enters delete.
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
	rec := &recordingActions{}
	m := newTestModel(t, rec, makeSession("alpha", false, "one"))

	if got := m.View(); got.AltScreen != true {
		t.Fatal("expected alt screen view")
	}

	// Filter to nothing and ensure the empty view still renders.
	m = feed(t, m, keyRune('z'))
	m = feed(t, m, keyRune('z'))
	m = feed(t, m, keyRune('z'))
	_ = m.View()
}

func TestFilteredTreeRowsAndTable(t *testing.T) {
	sessions := []Session{makeSession("alpha", true, "edit", "shell")}

	rows := filteredTreeRows(sessions, "", DefaultSortOptions().Window)
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

	// Narrow layout should still render (columns shrink to fit).
	narrow := buildPickerTableLayout(12)
	if narrow.row(rows[1]) == "" {
		t.Fatal("expected narrow row to render")
	}
}

func TestChooseTargetSuccess(t *testing.T) {
	orig := newPickerRunner
	defer func() { newPickerRunner = orig }()

	idx := 2
	newPickerRunner = func(m pickerModel) pickerRunner {
		m.selected = Target{SessionName: "alpha", WindowIndex: &idx}
		return staticRunner{model: m}
	}

	target, err := ChooseTarget(
		[]Session{makeSession("alpha", false, "one", "two")},
		DefaultSortOptions().Window,
		Actions{},
	)
	if err != nil {
		t.Fatalf("choose target: %v", err)
	}

	if target.SessionName != "alpha" || target.WindowIndex == nil || *target.WindowIndex != 2 {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestChooseTargetCancelled(t *testing.T) {
	orig := newPickerRunner
	defer func() { newPickerRunner = orig }()

	newPickerRunner = func(m pickerModel) pickerRunner {
		m.cancelled = true
		return staticRunner{model: m}
	}

	if _, err := ChooseTarget([]Session{makeSession("a", false, "w")}, nil, Actions{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestChooseTargetNoSelection(t *testing.T) {
	orig := newPickerRunner
	defer func() { newPickerRunner = orig }()

	newPickerRunner = func(m pickerModel) pickerRunner {
		return staticRunner{model: m} // nothing selected
	}

	if _, err := ChooseTarget([]Session{makeSession("a", false, "w")}, nil, Actions{}); err == nil {
		t.Fatal("expected no-selection error")
	}
}

type staticRunner struct {
	model pickerModel
}

func (s staticRunner) Run() (tea.Model, error) { return s.model, nil }
