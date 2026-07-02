package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

func runPicker(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("picker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	fzfEngine := flags.Bool("fzf-engine", false, "use fzf engine instead of built-in TUI")
	windows := flags.Bool("windows", false, "fzf engine: pick a window instead of a session")
	sessionSort := flags.String("session-sort", "", "session sort keys: field[:asc|desc],...")
	windowSort := flags.String("window-sort", "", "window sort keys: field[:asc|desc],...")
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
			pickerHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	// Validate flag combinations before any config loading, so invalid usage is
	// rejected deterministically rather than masked by an unrelated config error.
	if *windows && !*fzfEngine {
		writeErr(
			stderr,
			errors.New("--windows requires --fzf-engine (the TUI already lists windows)"),
		)

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

	sortOpts, err := app.ParsePickerSortOptions(*sessionSort, *windowSort)
	if err != nil {
		writeErr(stderr, fmt.Errorf("parse sort options: %w", err))

		return 1
	}

	tmuxApp := app.New(cfg)

	var target app.PickerTarget

	switch {
	case *fzfEngine && *windows:
		target, err = tmuxApp.SelectTargetWithFZFSorted(sortOpts)
		if err != nil {
			writeErr(stderr, fmt.Errorf("select target: %w", err))

			return 1
		}
	case *fzfEngine:
		session, err := tmuxApp.SelectWithFZFSorted(sortOpts)
		if err != nil {
			writeErr(stderr, fmt.Errorf("select target: %w", err))

			return 1
		}

		target = app.PickerTarget{SessionName: session}
	default:
		target, err = tmuxApp.SelectTargetWithTUISorted(sortOpts)
		if err != nil {
			writeErr(stderr, fmt.Errorf("select target: %w", err))

			return 1
		}

		// The interactive TUI shows a loading animation while the pick restores.
		err = tmuxApp.RestoreTargetAnimated(target)
		if err != nil {
			writeErr(stderr, fmt.Errorf("restore target: %w", err))

			return 1
		}

		return 0
	}

	// The fzf engine is also an interactive picker: attach into the pick even
	// from a plain shell (no animation — it has no TUI to draw into).
	err = tmuxApp.RestoreTargetInteractive(target)
	if err != nil {
		writeErr(stderr, fmt.Errorf("restore target: %w", err))

		return 1
	}

	return 0
}

func pickerHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux picker [flags]

Open session picker and restore selected session

Flags:
  -data-dir         snapshot directory
  -fzf-engine       use fzf engine instead of built-in TUI
  -windows          fzf engine: pick a window instead of a session
  -restore-timeout  max wait for restored pane commands to start (0 disables)
  -session-sort     session sort keys: field[:asc|desc],...
  -window-sort      window sort keys: field[:asc|desc],...
  -tmux-bin         tmux binary
`)
}
