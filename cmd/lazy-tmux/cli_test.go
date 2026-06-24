package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var out, errOut bytes.Buffer

	code := runCLI(args, &out, &errOut)

	return code, out.String(), errOut.String()
}

func TestCLINoArgsPrintsUsage(t *testing.T) {
	code, out, errOut := run(t)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}

	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage, got %q", out)
	}

	if errOut != "" {
		t.Fatalf("expected empty stderr, got %q", errOut)
	}
}

func TestCLIHelpVariants(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		code, out, _ := run(t, arg)
		if code != 0 {
			t.Fatalf("%s: expected exit 0, got %d", arg, code)
		}

		if !strings.Contains(out, "tmux session snapshots with lazy restore") {
			t.Fatalf("%s: expected help text, got %q", arg, out)
		}
	}
}

func TestCLIPerCommandHelp(t *testing.T) {
	for _, cmd := range []string{
		"save", "restore", "picker", "bootstrap", "daemon",
		"list", "setup", "wakeup", "sleep", "forget",
	} {
		code, out, _ := run(t, "help", cmd)
		if code != 0 {
			t.Fatalf("help %s: expected exit 0, got %d", cmd, code)
		}

		if !strings.Contains(strings.ToLower(out), "usage") {
			t.Fatalf("help %s: expected usage text, got %q", cmd, out)
		}
	}
}

func TestCLIHelpUnknownCommand(t *testing.T) {
	code, _, errOut := run(t, "help", "nope")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if !strings.Contains(errOut, "unknown command: nope") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	code, _, errOut := run(t, "wat")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if !strings.Contains(errOut, "unknown command: wat") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestCLISessionCommandsRequireSession(t *testing.T) {
	for _, cmd := range []string{"restore", "wakeup", "sleep"} {
		code, _, errOut := run(t, cmd)
		if code != 1 {
			t.Fatalf("%s: expected exit 1, got %d", cmd, code)
		}

		if !strings.Contains(errOut, "requires --session") {
			t.Fatalf("%s: expected requires --session, got %q", cmd, errOut)
		}
	}
}

func TestCLIFlagParseError(t *testing.T) {
	code, _, errOut := run(t, "save", "--definitely-not-a-flag")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if !strings.Contains(errOut, "parse flags") {
		t.Fatalf("expected parse flags error, got %q", errOut)
	}
}

func TestCLISaveScrollbackLinesValidation(t *testing.T) {
	code, _, errOut := run(
		t,
		"save",
		"--scrollback",
		"--scrollback-lines",
		"0",
		"--data-dir",
		t.TempDir(),
	)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	if !strings.Contains(errOut, "scrollback lines > 0") {
		t.Fatalf("expected scrollback lines validation, got %q", errOut)
	}
}

func TestCLISetupPrintsKeybinds(t *testing.T) {
	code, out, _ := run(t, "setup")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	for _, want := range []string{"lazy-tmux daemon", "display-popup", "lazy-tmux save"} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup output missing %q: %q", want, out)
		}
	}
}

func TestCLIListEmptyStore(t *testing.T) {
	code, out, _ := run(t, "list", "--data-dir", t.TempDir())
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty output for empty store, got %q", out)
	}
}

func TestCLIListPrintsSavedRecords(t *testing.T) {
	dir := t.TempDir()
	s := store.New(dir)

	err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "alpha",
		CapturedAt:  time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("seed alpha: %v", err)
	}

	code, out, _ := run(t, "list", "--data-dir", dir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	if !strings.Contains(out, "alpha") || !strings.Contains(out, "1w/1p") {
		t.Fatalf("unexpected list output: %q", out)
	}
}

func TestCLIReadsDataDirFromConfigFile(t *testing.T) {
	dir := t.TempDir()
	s := store.New(dir)

	err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "fromconfig",
		CapturedAt:  time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfgPath := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	if err := os.WriteFile(
		cfgPath,
		[]byte("data_dir = "+strconv.Quote(dir)+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LAZY_TMUX_CONFIG", cfgPath)

	// No --data-dir flag: the directory must come from the config file.
	code, out, _ := run(t, "list")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	if !strings.Contains(out, "fromconfig") {
		t.Fatalf("expected session from config data_dir, got %q", out)
	}
}

func TestCLIMalformedConfigFails(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	if err := os.WriteFile(cfgPath, []byte("not = valid = toml\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("LAZY_TMUX_CONFIG", cfgPath)

	code, _, errOut := run(t, "list")
	if code != 1 {
		t.Fatalf("expected exit 1 for malformed config, got %d", code)
	}

	if !strings.Contains(errOut, "load config") {
		t.Fatalf("expected load config error, got %q", errOut)
	}
}

func TestCLIBootstrapLastOnEmptyStore(t *testing.T) {
	code, _, errOut := run(t, "bootstrap", "--session", "last", "--data-dir", t.TempDir())
	if code != 0 {
		t.Fatalf("expected exit 0 on empty store, got %d (stderr=%q)", code, errOut)
	}
}

func TestCLIForgetMissingSessionSucceeds(t *testing.T) {
	// Forget is idempotent: removing a non-existent session is not an error.
	code, _, errOut := run(t, "forget", "--session", "ghost", "--data-dir", t.TempDir())
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr=%q)", code, errOut)
	}
}
