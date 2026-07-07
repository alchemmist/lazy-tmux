//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestViewFrameWidthInvariant(t *testing.T) {
	t.Parallel()

	rec := &recordingActions{}
	m := newTestModel(t, rec,
		makeSession("alpha", false, "edit", "shell", "logs"),
		makeSession("beta", true, "api"),
	)

	check := func(label string, mdl pickerModel) {
		lines := strings.Split(strings.TrimRight(mdl.View().Content, "\n"), "\n")
		want := ansi.StringWidth(lines[0])
		for i, ln := range lines {
			if got := ansi.StringWidth(ln); got != want {
				t.Fatalf("%s line %d width %d != %d:\n%q", label, i, got, want, ln)
			}
		}
		t.Logf("%s ok, %d lines @ %d cols", label, len(lines), want)
	}

	check("browse", m)
	check("delete", feed(t, m, keyAlt('d')))
	check("rename", feed(t, m, keyCtrl('r')))
	check("new", feed(t, m, keyAlt('n')))
	check("wake", feed(t, m, keyAlt('w')))

	pal := feed(t, m, keyRune('/'))
	check("palette", pal)

	withStatus := makeSession("live", true, "claude", "shell")
	withStatus.Statuses = map[int]WindowStatus{1: StatusWorking, 2: StatusAwaitingDecision}
	check("status", newTestModel(t, rec, withStatus))
}

func TestViewRendersStatusDot(t *testing.T) {
	t.Parallel()

	rec := &recordingActions{}

	sess := makeSession("live", true, "claude")
	sess.Statuses = map[int]WindowStatus{1: StatusWorking}

	m := newTestModel(t, rec, sess)

	if !strings.Contains(m.View().Content, workingSpinnerFrames[0]) {
		t.Fatal("expected the working spinner glyph in the rendered view")
	}

	plain := makeSession("plain", true, "vim")

	if strings.Contains(newTestModel(t, rec, plain).View().Content, workingSpinnerFrames[0]) {
		t.Fatal("window without a status must not render a glyph")
	}
}

func TestStatusGlyphFrameAnimatesWorking(t *testing.T) {
	t.Parallel()

	for i, frame := range workingSpinnerFrames {
		if got := statusGlyphFrame(StatusWorking, i); got != frame {
			t.Fatalf("frame %d: got %q want %q", i, got, frame)
		}
	}

	if got := statusGlyphFrame(
		StatusWorking,
		len(workingSpinnerFrames),
	); got != workingSpinnerFrames[0] {
		t.Fatal("frame index must wrap around")
	}

	if got := statusGlyphFrame(StatusIdle, 3); got != glyphIdle {
		t.Fatalf("non-working status must stay static, got %q", got)
	}
}
