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

	if !strings.Contains(m.View().Content, glyphWorking) {
		t.Fatal("expected a working status glyph in the rendered view")
	}

	plain := makeSession("plain", true, "vim")

	if strings.Contains(newTestModel(t, rec, plain).View().Content, glyphWorking) {
		t.Fatal("window without a status must not render a glyph")
	}
}
