//go:build !lazy_fzf

package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestViewFrameWidthInvariant(t *testing.T) {
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
}
