package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

func runDaemon(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdDaemon, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	interval := flags.Duration("interval", 0, "autosave interval")
	scrollback := flags.Bool("scrollback", false, "capture shell pane scrollback")
	scrollbackLines := flags.Int(
		"scrollback-lines",
		config.DefaultScrollbackLines,
		"max shell scrollback lines per pane",
	)
	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			daemonHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}

	applyDirBinOverrides(flags, &cfg, dataDir, tmuxBin)

	if flagPassed(flags, "interval") {
		cfg.SaveInterval = *interval
	}

	err = applyScrollbackOverrides(flags, &cfg, scrollback, scrollbackLines)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	a := app.New(cfg)

	err = a.RunDaemon(cfg.SaveInterval)
	if err != nil {
		writeErr(stderr, fmt.Errorf("run daemon: %w", err))

		return 1
	}

	return 0
}

func daemonHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux daemon [flags]

Periodically save all sessions

Flags:
  -data-dir          snapshot directory
  -interval         autosave interval (default 0s)
  -scrollback       capture shell pane scrollback
  -scrollback-lines int   max shell scrollback lines per pane (default 5000)
  -tmux-bin         tmux binary
`)
}
