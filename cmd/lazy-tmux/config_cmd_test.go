package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIConfigGenAndShow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazy-tmux.toml")

	// gen writes the file.
	if code, out, errOut := run(t, "config", "gen", "--path", path); code != 0 {
		t.Fatalf("config gen: exit %d stderr=%q", code, errOut)
	} else if !strings.Contains(out, "wrote config to "+path) {
		t.Fatalf("config gen output: %q", out)
	}

	// gen again without --force must refuse.
	if code, _, _ := run(t, "config", "gen", "--path", path); code != 1 {
		t.Fatalf("config gen over existing should fail, got exit %d", code)
	}

	// show reads that file and prints the source + effective values.
	t.Setenv("LAZY_TMUX_CONFIG", path)

	code, out, _ := run(t, "config", "show")
	if code != 0 {
		t.Fatalf("config show: exit %d", code)
	}

	if !strings.Contains(out, "config source: "+path) || !strings.Contains(out, "tmux_bin") {
		t.Fatalf("config show output: %q", out)
	}
}

func TestCLIConfigRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"config", "gen", "extra"}, {"config", "show", "extra"}} {
		code, _, errOut := run(t, args...)
		if code != 1 {
			t.Fatalf("%v: expected exit 1, got %d", args, code)
		}

		if !strings.Contains(errOut, "unexpected arguments") {
			t.Fatalf("%v: expected unexpected-args error, got %q", args, errOut)
		}
	}
}

func TestCLIConfigUnknownSubcommand(t *testing.T) {
	t.Parallel()

	code, _, errOut := run(t, "config", "bogus")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if !strings.Contains(errOut, "unknown config subcommand: bogus") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestCLIConfigHelp(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"help", "config"}, {"config", "-h"}, {"config"}} {
		_, outOrErr := captureConfig(t, args)
		if !strings.Contains(outOrErr, "lazy-tmux config <subcommand>") {
			t.Fatalf("%v: expected config help, got %q", args, outOrErr)
		}
	}
}

// captureConfig runs args and returns the combined stdout+stderr, since bare
// `config` prints help to stderr while `help config` / `config -h` use stdout.
func captureConfig(t *testing.T, args []string) (int, string) {
	t.Helper()

	code, out, errOut := run(t, args...)

	return code, out + errOut
}
