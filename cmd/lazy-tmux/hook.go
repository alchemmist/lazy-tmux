package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
)

// runHook dispatches the hook writers invoked by external programs (e.g. Claude
// Code). These run inside another tool's hook pipeline, so they must be fast and
// must not fail the host on transient errors.
func runHook(args []string, _, stderr io.Writer) int {
	if len(args) == 0 {
		writeErr(stderr, errors.New("usage: lazy-tmux hook claude-status --state <state>"))

		return exitUsage
	}

	switch args[0] {
	case "claude-status":
		return runClaudeStatusHook(args[1:], os.Stdin, stderr)
	default:
		writeErr(stderr, fmt.Errorf("unknown hook %q", args[0]))

		return exitUsage
	}
}

// runClaudeStatusHook records the Claude pane's live state. Claude passes the
// hook payload (session_id, cwd) as JSON on stdin; cwd falls back to the
// process working directory (the hook runs in the pane's cwd).
func runClaudeStatusHook(args []string, stdin io.Reader, stderr io.Writer) int {
	flags := flag.NewFlagSet("claude-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	state := flags.String("state", "", "status: working|awaiting_decision|awaiting_input|idle")

	err := flags.Parse(args)
	if err != nil {
		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return exitUsage
	}

	if !claude.ValidState(*state) {
		writeErr(stderr, fmt.Errorf("invalid --state %q", *state))

		return exitUsage
	}

	var payload struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}

	if stdin != nil {
		_ = json.NewDecoder(stdin).Decode(&payload) // best-effort; empty is fine
	}

	cwd := payload.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	cfg, err := config.Load()
	if err != nil {
		return 0 // never break the host hook pipeline
	}

	_ = claude.WriteStatus(
		app.ClaudeStatusDir(cfg.DataDir),
		cwd,
		*state,
		payload.SessionID,
		time.Now(),
	)

	return 0
}

func hookHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux hook claude-status --state <state>

Record a program's live status (invoked from its hooks). States for claude-status:
working, awaiting_decision, awaiting_input, idle.
`)
}
