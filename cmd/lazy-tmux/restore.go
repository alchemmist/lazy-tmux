package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

func runRestore(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	session := flags.String("session", "", "session to restore")
	switchClient := flags.Bool("switch", true, "switch active client to restored session")
	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			restoreHelp(stdout)
			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if strings.TrimSpace(*session) == "" {
		writeErr(stderr, fmt.Errorf("restore requires --session"))
		return 1
	}

	cfg := config.Default()
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	if *tmuxBin != "" {
		cfg.TmuxBin = *tmuxBin
	}

	tmuxApp := app.New(cfg)

	err = tmuxApp.Restore(strings.TrimSpace(*session), *switchClient)
	if err != nil {
		writeErr(stderr, fmt.Errorf("restore session: %w", err))
		return 1
	}

	return 0
}

func restoreHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux restore [flags]

Restore one session from disk

Flags:
  -data-dir     snapshot directory
  -session      session to restore
  -switch       switch active client to restored session (default true)
  -tmux-bin     tmux binary
`)
}
