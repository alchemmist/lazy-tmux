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

type sessionOp func(*app.App, string) error

func runSessionOp(args []string, stdout, stderr io.Writer, name string, operation sessionOp) int {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	session := flags.String("session", "", name+" target session")
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
			printSessionHelp(name, stdout)
			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if strings.TrimSpace(*session) == "" {
		writeErr(stderr, fmt.Errorf("%s requires --session", name))
		return 1
	}

	cfg := config.Default()
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	if *tmuxBin != "" {
		cfg.TmuxBin = *tmuxBin
	}

	cfg.RestoreTimeout = *restoreTimeout

	a := app.New(cfg)

	err = operation(a, strings.TrimSpace(*session))
	if err != nil {
		writeErr(stderr, fmt.Errorf("%s session: %w", name, err))
		return 1
	}

	return 0
}

func printSessionHelp(name string, writer io.Writer) {
	desc := map[string]string{
		"wakeup": "Restore a saved session without switching clients",
		"forget": "Remove a stored session from disk",
	}

	restoreTimeoutHelp := ""
	if name == "wakeup" {
		restoreTimeoutHelp = "\n  -restore-timeout  max wait for restored pane commands to start (0 disables)"
	}

	_, _ = fmt.Fprintf(writer, `Usage: lazy-tmux %s [flags]

%s

Flags:
  -data-dir     snapshot directory
  -session      %s target session
  -tmux-bin     tmux binary%s
`, name, desc[name], name, restoreTimeoutHelp)
}
