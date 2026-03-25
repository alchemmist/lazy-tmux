//go:build !lazy_fzf

package picker

import "testing"

func TestTuiDisabledFalse(t *testing.T) {
	if tuiDisabled() {
		t.Fatalf("expected TUI to be enabled in this build")
	}
}
