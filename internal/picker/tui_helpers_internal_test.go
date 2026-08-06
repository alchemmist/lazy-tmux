//go:build !lazy_fzf

package picker

import "testing"

func TestFuzzyMatch(t *testing.T) {
	t.Parallel()

	if !fuzzyMatch("abc", "axbxc") {
		t.Fatal("expected subsequence match")
	}

	if !fuzzyMatch("abc", "acb") {
		t.Fatal("expected transposed letters to match")
	}

	if fuzzyMatch("abc", "cab") {
		t.Fatal("expected unrelated ordering not to match")
	}

	if !fuzzyMatch("", "anything") {
		t.Fatal("empty query matches everything")
	}
}

func TestFuzzyScorePrioritizesStrongMatches(t *testing.T) {
	t.Parallel()

	exact, _ := fuzzyScore("ci", "ci")
	prefix, _ := fuzzyScore("ci", "ci-tools")
	substring, _ := fuzzyScore("ci", "my-ci-tools")
	subsequence, _ := fuzzyScore("ci", "codex integration")

	if exact <= prefix || prefix <= substring || substring <= subsequence {
		t.Fatalf(
			"unexpected score order: exact=%d prefix=%d substring=%d subsequence=%d",
			exact,
			prefix,
			substring,
			subsequence,
		)
	}

	if _, ok := fuzzyScore("codxe", "codex"); !ok {
		t.Fatal("expected a transposition typo to match")
	}
}

func TestSessionStateIcon(t *testing.T) {
	t.Parallel()

	restored := sessionStateIcon(true)
	notRestored := sessionStateIcon(false)

	if restored == "" || notRestored == "" {
		t.Fatalf(
			"both state icons should be non-empty: restored=%q notRestored=%q",
			restored,
			notRestored,
		)
	}

	if restored == notRestored {
		t.Fatalf("restored and not-restored icons should differ, both %q", restored)
	}
}

func TestTruncateString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"hello", 2, "he"},
		{"hello", -1, ""},
	}

	for _, c := range cases {
		if got := truncateString(c.in, c.max); got != c.want {
			t.Fatalf("truncate(%q,%d)=%q want %q", c.in, c.max, got, c.want)
		}
	}
}
