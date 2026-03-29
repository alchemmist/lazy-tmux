//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type fakePickerRunner struct {
	result tea.Model
	err    error
}

func (f fakePickerRunner) Run() (tea.Model, error) {
	return f.result, f.err
}

func TestNewPickerModelInitializesRows(t *testing.T) {
	sessions := []Session{
		{
			Record:  snapshot.Record{SessionName: "demo"},
			Windows: []snapshot.Window{{Index: 0, Name: "one"}},
		},
	}

	m := newPickerModel(sessions, DefaultSortOptions().Window, Actions{})
	if len(m.visible) == 0 {
		t.Fatal("expected visible rows")
	}
}

func TestPickerModelInitReturnsBlink(t *testing.T) {
	m := baseModelForTests()
	if m.Init() == nil {
		t.Fatal("expected init cmd")
	}
}

func TestPickerModelUpdateWindowSize(t *testing.T) {
	m := baseModelForTests()
	m.viewport.SetWidth(10)
	m.viewport.SetHeight(5)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})

	out := next.(pickerModel)
	if out.width != 30 || out.height != 8 {
		t.Fatalf("unexpected size %dx%d", out.width, out.height)
	}
}

func TestPickerModelViewRendersRows(t *testing.T) {
	model := baseModelForTests()
	model.visible = []pickerRow{
		{target: Target{SessionName: "demo"}, selectable: true, item: "demo"},
	}
	model.cursor = 0
	model.width = 60
	model.height = 10
	model.viewport.SetWidth(40)
	model.viewport.SetHeight(3)
	model.renderViewport()

	view := model.View()
	if !strings.Contains(view.Content, "demo") {
		t.Fatalf("expected demo row, got %s", view.Content)
	}
}

func TestChooseTargetUsesRunner(t *testing.T) {
	orig := newPickerRunner
	defer func() { newPickerRunner = orig }()

	newPickerRunner = func(m pickerModel) pickerRunner {
		m.selected = Target{SessionName: "demo"}
		return fakePickerRunner{result: m}
	}

	target, err := ChooseTarget(nil, nil, Actions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.SessionName != "demo" {
		t.Fatalf("expected demo target, got %v", target)
	}
}

func TestEnsureCursorVisibleMovesWindow(t *testing.T) {
	model := baseModelForTests()
	model.visible = make([]pickerRow, 5)

	for i := range model.visible {
		model.visible[i].selectable = true
	}

	model.viewport.SetHeight(2)
	model.viewport.SetYOffset(0)
	model.cursor = 4
	model.renderViewport()
	model.ensureCursorVisible()

	if model.viewport.YOffset() == 0 {
		t.Fatalf("expected viewport offset to rise, got %d", model.viewport.YOffset())
	}
}
