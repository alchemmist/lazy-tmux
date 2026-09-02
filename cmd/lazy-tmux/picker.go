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
	flags := flag.NewFlagSet(cmdPicker, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	fzfEngine := flags.Bool("fzf-engine", false, "use fzf engine instead of built-in TUI")
	windows := flags.Bool("windows", false, "fzf engine: pick a window instead of a session")
	sessionsOnly := flags.Bool("sessions-only", false, "open the fast sessions-only TUI")
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

	err = validatePickerEngineFlags(*fzfEngine, *windows, *sessionsOnly)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}

	applyDirBinOverrides(flags, &cfg, dataDir, tmuxBin)

	if flagPassed(flags, "restore-timeout") {
		cfg.RestoreTimeout = *restoreTimeout
	}

	sortOpts, err := app.ParsePickerSortOptions(*sessionSort, *windowSort)
	if err != nil {
		writeErr(stderr, fmt.Errorf("parse sort options: %w", err))

		return 1
	}

	tmuxApp := app.New(cfg)

	return runConfiguredPicker(tmuxApp, *fzfEngine, *windows, *sessionsOnly, sortOpts, stderr)
}

func runConfiguredPicker(
	tmuxApp *app.App,
	fzfEngine, windows, sessionsOnly bool,
	sortOpts app.PickerSortOptions,
	stderr io.Writer,
) int {
	if fzfEngine {
		return runFZFPicker(tmuxApp, windows, sortOpts, stderr)
	}
	if sessionsOnly {
		return runQuickSessionPicker(tmuxApp, stderr)
	}

	return runTUIPicker(tmuxApp, sortOpts, stderr)
}

func validatePickerEngineFlags(fzfEngine, windows, sessionsOnly bool) error {
	if windows && !fzfEngine {
		return errWindowsRequiresFzf
	}
	if sessionsOnly && fzfEngine {
		return errSessionsOnlyRequiresTUI
	}

	return nil
}

func runQuickSessionPicker(
	tmuxApp *app.App,
	stderr io.Writer,
) int {
	session, err := tmuxApp.SelectQuickSessionWithTUI()
	if err != nil {
		writeErr(stderr, fmt.Errorf("select session: %w", err))

		return 1
	}

	err = tmuxApp.OpenQuickSession(session)
	if err != nil {
		writeErr(stderr, fmt.Errorf("restore target: %w", err))

		return 1
	}

	return 0
}

func runTUIPicker(tmuxApp *app.App, sortOpts app.PickerSortOptions, stderr io.Writer) int {
	for {
		target, err := tmuxApp.SelectTargetWithTUISorted(sortOpts)
		if err != nil {
			writeErr(stderr, fmt.Errorf("select target: %w", err))

			return 1
		}

		cancelled, err := tmuxApp.RestoreTargetAnimated(target)
		if err != nil {
			writeErr(stderr, fmt.Errorf("restore target: %w", err))

			return 1
		}

		if cancelled {
			continue
		}

		return 0
	}
}

func runFZFPicker(
	tmuxApp *app.App,
	windows bool,
	sortOpts app.PickerSortOptions,
	stderr io.Writer,
) int {
	var (
		target app.PickerTarget
		err    error
	)

	if windows {
		target, err = tmuxApp.SelectTargetWithFZFSorted(sortOpts)
	} else {
		var session string

		session, err = tmuxApp.SelectWithFZFSorted(sortOpts)
		target = app.PickerTarget{SessionName: session}
	}

	if err != nil {
		writeErr(stderr, fmt.Errorf("select target: %w", err))

		return 1
	}

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
  -sessions-only    open the fast sessions-only TUI
  -session-sort     session sort keys: field[:asc|desc],...
  -window-sort      window sort keys: field[:asc|desc],...
  -tmux-bin         tmux binary
`)
}
