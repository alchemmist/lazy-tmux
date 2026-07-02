//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestLoadingViewHiddenBeforeGrace(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

	// The animation lives inside the picker's rounded frame.
	for _, corner := range []string{"╭", "╮", "╰", "╯", "│"} {
		if !strings.Contains(view.Content, corner) {
			t.Fatalf("expected the rounded frame char %q around the animation", corner)
		}
	}

	// Every framed line must be the same display width, or the border misaligns.
	want := ansi.StringWidth(lines[0])
	for i, ln := range lines {
		if got := ansi.StringWidth(ln); got != want {
			t.Fatalf("line %d width %d != %d: %q", i, got, want, ln)
		}
	}
}

func TestLoadingQuitsWhenRestoreDone(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	m := newLoadingModel("x", make(chan struct{}))

	updated, cmd := m.Update(graceMsg{})

	lm, ok := updated.(loadingModel)
	if !ok {
		t.Fatalf("expected loadingModel, got %T", updated)
	}

	if !lm.started {
		t.Fatal("graceMsg must reveal the field (started=true)")
	}

	if cmd == nil {
		t.Fatal("graceMsg must start the frame ticker")
	}
}
