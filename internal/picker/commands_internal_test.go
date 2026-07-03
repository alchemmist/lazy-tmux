//go:build !lazy_fzf

package picker

import "testing"

func TestMatchCommands(t *testing.T) {
	t.Parallel()

	if got := matchCommands(""); len(got) != len(pickerCommands) {
		t.Fatalf("empty prefix should match every command, got %d", len(got))
	}

	got := matchCommands("de")
	if len(got) != 1 || got[0].name != "delete" {
		t.Fatalf("prefix 'de' should match only delete, got %+v", got)
	}

	if got := matchCommands("zzz"); len(got) != 0 {
		t.Fatalf("unknown prefix should match nothing, got %+v", got)
	}

	// Leading slash is stripped by the caller, but a stray space must not break
	// matching.
	if got := matchCommands(" wake "); len(got) != 1 || got[0].mode != actionWake {
		t.Fatalf("padded 'wake' should match wake, got %+v", got)
	}
}

func TestCommandForMode(t *testing.T) {
	t.Parallel()

	cmd, ok := commandForMode(actionRename)
	if !ok || cmd.name != "rename" || cmd.accent != colRename {
		t.Fatalf("rename mode should resolve to the rename command, got %+v ok=%v", cmd, ok)
	}

	if _, ok := commandForMode(actionBrowse); ok {
		t.Fatal("browse has no backing command")
	}
}

func TestAccentForMode(t *testing.T) {
	t.Parallel()

	if accentForMode(actionBrowse) != colAccent {
		t.Fatal("browse should use the amber accent")
	}

	if accentForMode(actionDelete) != colDelete {
		t.Fatal("delete should use the red accent")
	}

	if accentForMode(actionWake) != accentForMode(actionSleep) {
		t.Fatal("wake and sleep share the cyan accent")
	}
}
