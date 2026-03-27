//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func TestPickerModelUpdateMovesCursor(t *testing.T) {
	m := pickerModel{
		visible: []pickerRow{
			{item: "one", selectable: true},
			{item: "two", selectable: true},
		},
		cursor:     0,
		mode:       modeBrowse,
		viewport:   viewport.New(),
		queryInput: textinput.New(),
		width:      80,
		height:     20,
	}
	m.viewport.SetWidth(78)
	m.viewport.SetHeight(10)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})

	out := next.(pickerModel)
	if out.cursor != 1 {
		t.Fatalf("expected cursor to move to 1, got %d", out.cursor)
	}
}

func TestPickerModelUpdateCancel(t *testing.T) {
	m := pickerModel{
		mode:       modeBrowse,
		viewport:   viewport.New(),
		queryInput: textinput.New(),
		width:      80,
		height:     20,
	}
	m.viewport.SetWidth(78)
	m.viewport.SetHeight(10)

	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	out := next.(pickerModel)
	if !out.cancelled {
		t.Fatal("expected cancelled to be true")
	}
}

func TestPickerModelViewNoVisible(t *testing.T) {
	m := pickerModel{
		mode:       modeBrowse,
		visible:    nil,
		queryInput: textinput.New(),
		viewport:   viewport.New(),
	}

	view := m.View()
	if !strings.Contains(view.Content, "No sessions or windows match query") {
		t.Fatalf("unexpected view output: %s", view.Content)
	}
}
