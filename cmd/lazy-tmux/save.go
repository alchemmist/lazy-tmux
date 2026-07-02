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

func runSave(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdSave, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	all := flags.Bool("all", false, "save all sessions")
	session := flags.String("session", "", "save specific session")
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
			saveHelp(stdout)

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

	if flagPassed(flags, "scrollback") {
		cfg.Scrollback.Enabled = *scrollback
	}

	if flagPassed(flags, "scrollback-lines") {
		cfg.Scrollback.Lines = *scrollbackLines
	}

	if cfg.Scrollback.Enabled && cfg.Scrollback.Lines <= 0 {
		writeErr(stderr, errors.New("scrollback requires scrollback lines > 0"))

		return 1
	}

	tmuxApp := app.New(cfg)

	switch {
	case *all:
		var saved int
		saved, err = tmuxApp.SaveAll()
		if err == nil {
			if saved == 0 {
				// Don't leave the user staring at silence (issue #125): a likely
				// cause is lazy-tmux talking to a different tmux than theirs
				// (e.g. tmux is a shell alias) — point them at tmux_bin.
				_, _ = fmt.Fprintln(stdout,
					"no running tmux sessions found "+
						"(if you do have sessions, set tmux_bin / --tmux-bin to your tmux binary)")
			} else {
				_, _ = fmt.Fprintf(stdout, "saved %d session(s)\n", saved)
			}
		}
	case strings.TrimSpace(*session) != "":
		err = tmuxApp.SaveSession(strings.TrimSpace(*session))
	default:
		err = tmuxApp.SaveCurrent()
	}

	if err != nil {
		writeErr(stderr, fmt.Errorf("save session: %w", err))

		return 1
	}

	return 0
}

func saveHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux save [flags]

Save current or selected sessions

Flags:
  -all              save all sessions
  -data-dir         snapshot directory
  -scrollback       capture shell pane scrollback
  -scrollback-lines int   max shell scrollback lines per pane (default 5000)
  -session         save specific session
  -tmux-bin        tmux binary
`)
}
