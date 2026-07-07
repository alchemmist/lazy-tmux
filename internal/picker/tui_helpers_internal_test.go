//go:build !lazy_fzf

package picker

import "testing"

func TestFuzzyMatch(t *testing.T) {
	t.Parallel()

	if !fuzzyMatch("abc", "axbxc") {
		t.Fatal("expected subsequence match")
	}

	if fuzzyMatch("abc", "acb") {
		t.Fatal("expected order-sensitive non-match")
	}

	if !fuzzyMatch("", "anything") {
		t.Fatal("empty query matches everything")
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
