package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
)

func runHook(args []string, _, stderr io.Writer) int {
	if len(args) == 0 {
		writeErr(stderr, errHookUsage)

		return 1
	}

	switch args[0] {
	case "claude-status":
		return runClaudeStatusHook(args[1:], os.Stdin, stderr)
	case "theme":
		return runThemeHook(args[1:], stderr)
	default:
		writeErr(stderr, fmt.Errorf("%w %q", errUnknownHook, args[0]))

		return 1
	}
}

func runThemeHook(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("theme", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	theme := flags.String("theme", "", "theme: dark|light")

	if err := flags.Parse(args); err != nil {
		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && *theme != "") {
		writeErr(stderr, errUnexpectedArguments)

		return 1
	}
	if flags.NArg() == 1 {
		*theme = flags.Arg(0)
	}
	if !config.IsValidTheme(*theme) {
		writeErr(stderr, fmt.Errorf("invalid theme %q (want dark or light)", *theme))

		return 1
	}
	if err := config.SetTheme(config.DefaultConfigPath(), *theme); err != nil {
		writeErr(stderr, fmt.Errorf("set theme: %w", err))

		return 1
	}

	return 0
}

func runClaudeStatusHook(args []string, stdin io.Reader, stderr io.Writer) int {
	flags := flag.NewFlagSet("claude-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	state := flags.String("state", "", "status: working|awaiting_decision|awaiting_input|idle")

	err := flags.Parse(args)
	if err != nil {
		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if !claude.ValidState(*state) {
		writeErr(stderr, fmt.Errorf("%w %q", errInvalidHookState, *state))

		return 1
	}

	var payload struct {
		SessionID string `json:"session_id"`
		CWD       string `json:"cwd"`
	}

	if stdin != nil {
		_ = json.NewDecoder(stdin).Decode(&payload)
	}

	cwd := payload.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	cfg, err := config.Load()
	if err != nil {
		return 0
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
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux hook <name> [flags]

Record a program's live status (invoked from its hooks). States for claude-status:
working, awaiting_decision, awaiting_input, idle.

Theme hook (usable from a daemon or external automation):
  lazy-tmux hook theme dark|light
  lazy-tmux hook theme --theme dark|light
`)
}
