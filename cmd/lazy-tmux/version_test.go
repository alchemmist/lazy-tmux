package main

import (
	"strings"
	"testing"
)

func TestCLIVersionVariants(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"version", "-v", "--version"} {
		code, out, errOut := run(t, arg)
		if code != 0 {
			t.Fatalf("%s: expected exit 0, got %d", arg, code)
		}

		if !strings.HasPrefix(out, "lazy-tmux ") {
			t.Fatalf("%s: expected 'lazy-tmux <version>' line, got %q", arg, out)
		}

		if errOut != "" {
			t.Fatalf("%s: expected empty stderr, got %q", arg, errOut)
		}
	}
}

func TestCLIVersionHelp(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"help", "version"}, {"version", "-h"}, {"version", "--help"}} {
		code, out, _ := run(t, args...)
		if code != 0 {
			t.Fatalf("%v: expected exit 0, got %d", args, code)
		}

		if !strings.Contains(out, "Usage: lazy-tmux version") {
			t.Fatalf("%v: expected version help, got %q", args, out)
		}
	}
}

//nolint:paralleltest // writes the global version read by parallel tests
func TestResolveVersionPrefersLdflags(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "1.2.3"
	if got := resolveVersion(); got != "1.2.3" {
		t.Fatalf("expected injected version, got %q", got)
	}
}

//nolint:paralleltest // writes the global version read by parallel tests
func TestResolveVersionFallbackNonEmpty(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = ""
	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion returned empty without an injected version")
	}
}
