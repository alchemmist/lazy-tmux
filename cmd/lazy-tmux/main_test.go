package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run(nil, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage in stdout, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "lazy-tmux - tmux session snapshots with lazy restore") {
		t.Fatalf("expected help text, got: %s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", errOut.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"wat"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "unknown command: wat") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunRestoreRequiresSession(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"restore"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "restore requires --session") {
		t.Fatalf("unexpected stderr: %s", errOut.String())
	}
}

func TestRunBootstrapLastOnEmptyStore(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	dir := t.TempDir()

	code := run([]string{"bootstrap", "--session", "last", "--data-dir", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}
}

func TestRunListPrintsSavedRecords(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	dir := t.TempDir()
	s := store.New(dir)

	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "alpha",
		CapturedAt:  time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}}}},
	}); err != nil {
		t.Fatalf("save alpha: %v", err)
	}
	if err := s.SaveSession(snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: "beta",
		CapturedAt:  time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
		Windows:     []snapshot.Window{{Index: 0, Panes: []snapshot.Pane{{Index: 0}, {Index: 1}}}},
	}); err != nil {
		t.Fatalf("save beta: %v", err)
	}

	code := run([]string{"list", "--data-dir", dir}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", code, errOut.String())
	}

	text := out.String()
	if !strings.Contains(text, "alpha\t") || !strings.Contains(text, "beta\t") {
		t.Fatalf("expected both records in output, got:\n%s", text)
	}
}
