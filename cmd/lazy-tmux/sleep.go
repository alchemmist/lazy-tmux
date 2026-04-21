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

func runSleep(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sleep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	session := flags.String("session", "", "session to sleep")
	scrollback := flags.Bool("scrollback", false, "capture shell pane scrollback")
	scrollbackLines := flags.Int("scrollback-lines", 5000, "max shell scrollback lines per pane")
	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			sleepHelp(stdout)
			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if strings.TrimSpace(*session) == "" {
		writeErr(stderr, fmt.Errorf("sleep requires --session"))
		return 1
	}

	if *scrollback && *scrollbackLines <= 0 {
		writeErr(
			stderr,
			fmt.Errorf("sleep requires --scrollback-lines > 0 when --scrollback is enabled"),
		)

		return 1
	}

	cfg := config.Default()
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	}

	if *tmuxBin != "" {
		cfg.TmuxBin = *tmuxBin
	}

	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines

	a := app.New(cfg)

	err = a.Sleep(strings.TrimSpace(*session))
	if err != nil {
		writeErr(stderr, fmt.Errorf("sleep session: %w", err))
		return 1
	}

	return 0
}

func sleepHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux sleep [flags]

Save and close a running session

Flags:
  -data-dir          snapshot directory
  -scrollback       capture shell pane scrollback
  -scrollback-lines int   max shell scrollback lines per pane (default 5000)
  -session          session to sleep
  -tmux-bin         tmux binary
`)
}
