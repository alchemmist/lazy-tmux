package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
)

func runDaemon(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	interval := flags.Duration("interval", 0, "autosave interval")
	scrollback := flags.Bool("scrollback", false, "capture shell pane scrollback")
	scrollbackLines := flags.Int("scrollback-lines", 5000, "max shell scrollback lines per pane")
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

	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	if *tmuxBin != "" {
		cfg.TmuxBin = *tmuxBin
	}

	if flagPassed(flags, "interval") {
		cfg.SaveInterval = *interval
	}

	if flagPassed(flags, "scrollback") {
		cfg.Scrollback.Enabled = *scrollback
	}

	if flagPassed(flags, "scrollback-lines") {
		cfg.Scrollback.Lines = *scrollbackLines
	}

	if cfg.Scrollback.Enabled && cfg.Scrollback.Lines <= 0 {
		writeErr(stderr, fmt.Errorf("scrollback requires scrollback lines > 0"))
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
