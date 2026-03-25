//go:build !lazy_fzf

package picker

import "testing"

func TestTrim(t *testing.T) {
	if got := trim("hello", -1); got != "" {
		t.Fatalf("expected trim with negative n to be empty, got %q", got)
	}
	if got := trim("hello", 0); got != "" {
		t.Fatalf("expected trim with n=0 to be empty, got %q", got)
	}
	if got := trim("hello", 3); got != "hel" {
		t.Fatalf("expected trim with n=3 to keep prefix without ellipsis, got %q", got)
	}
	if got := trim("hello", 4); got != "h..." {
		t.Fatalf("expected trim with n=4 to use ellipsis, got %q", got)
	}
	if got := trim("hi", 5); got != "hi" {
		t.Fatalf("expected trim to return original when shorter, got %q", got)
	}
}

