package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

func runBootstrap(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdBootstrap, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	session := flags.String("session", "last", "session name or 'last'")
	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")
	restoreTimeout := flags.Duration(
		"restore-timeout",
		config.Default().RestoreTimeout,
		"max wait for restored pane commands to start (0 disables)",
	)

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			bootstrapHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}

	if flagPassed(flags, "data-dir") {
		cfg.DataDir = *dataDir
	}

	if flagPassed(flags, "tmux-bin") {
		cfg.TmuxBin = *tmuxBin
	}

	if flagPassed(flags, "restore-timeout") {
		cfg.RestoreTimeout = *restoreTimeout
	}

	a := app.New(cfg)

	err = a.Bootstrap(*session)
	if err != nil {
		writeErr(stderr, fmt.Errorf("bootstrap session: %w", err))

		return 1
	}

	return 0
}

func bootstrapHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux bootstrap [flags]

Restore one session at tmux startup

Flags:
  -data-dir         snapshot directory
  -restore-timeout  max wait for restored pane commands to start (0 disables)
  -session          session name or 'last' (default "last")
  -tmux-bin         tmux binary
`)
}
