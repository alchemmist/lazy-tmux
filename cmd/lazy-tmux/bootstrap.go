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
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	session := flags.String("session", "last", "session name or 'last'")
	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			bootstrapHelp(stdout)
			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	cfg := config.Default()
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	if *tmuxBin != "" {
		cfg.TmuxBin = *tmuxBin
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
  -data-dir     snapshot directory
  -session      session name or 'last' (default "last")
  -tmux-bin     tmux binary
`)
}
