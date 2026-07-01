//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLoadingViewHiddenBeforeGrace(t *testing.T) {
	m := newLoadingModel("blog", make(chan struct{}))
	m.width, m.height = 80, 24

	// Before the grace period (started=false) nothing renders, and we stay off
	// the alt screen so a fast restore never flashes.
	view := m.View()
	if strings.TrimSpace(view.Content) != "" {
		t.Fatalf("expected empty pre-grace view, got %q", view.Content)
	}

	if view.AltScreen {
		t.Fatal("pre-grace view must not enter the alt screen")
	}
}

func TestLoadingViewRendersFieldAndCaption(t *testing.T) {
	m := newLoadingModel("blog", make(chan struct{}))
	m.width, m.height = 80, 24
	m.started = true
	m.frame = 10

	view := m.View()
	if !view.AltScreen {
		t.Fatal("animated view should use the alt screen")
	}

	lines := strings.Split(view.Content, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected 24 rows, got %d", len(lines))
	}

	if !strings.Contains(view.Content, "restoring blog…") {
		t.Fatal("expected the centered restoring caption")
	}

	// The field should contain at least one non-space ramp glyph somewhere.
	if !strings.ContainsAny(view.Content, ".·:-=+*#%@") {
		t.Fatal("expected the ascii field to render ramp glyphs")
	}
}

func TestLoadingQuitsWhenRestoreDone(t *testing.T) {
	m := newLoadingModel("x", make(chan struct{}))

	_, cmd := m.Update(restoreDoneMsg{})
	if cmd == nil {
		t.Fatal("restoreDoneMsg must return a command")
	}

	if msg := cmd(); msg == nil {
		t.Fatal("expected a quit message")
	} else if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestLoadingGraceRevealsField(t *testing.T) {
	m := newLoadingModel("x", make(chan struct{}))

	updated, cmd := m.Update(graceMsg{})
	if !updated.(loadingModel).started {
		t.Fatal("graceMsg must reveal the field (started=true)")
	}

	if cmd == nil {
		t.Fatal("graceMsg must start the frame ticker")
	}
}
