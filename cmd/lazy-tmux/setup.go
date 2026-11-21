package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

func runSetup(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdSetup, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			setupHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	const daemonCmd = "run-shell -b 'lazy-tmux daemon --interval 3m --scrollback " +
		">/tmp/lazy-tmux.log 2>&1 || tmux display-message \"lazy-tmux daemon already running\"'"

	const popupKey = "bind-key f display-popup -B -E 'lazy-tmux picker'"

	// Generate the hash at shell runtime so run-shell cannot expand it as a tmux format.
	const saveKey = `bind-key C-s run-shell 'hash=$(printf "\043"); ` +
		`msg=$(lazy-tmux save --all --scrollback 2>&1 | tr "\n" " " | sed "s/$hash/$hash$hash/g"); ` +
		`tmux display-message -- "$msg"'`

	_, _ = fmt.Fprintln(stdout, daemonCmd)
	_, _ = fmt.Fprintln(stdout, popupKey)
	_, _ = fmt.Fprintln(stdout, saveKey)

	return 0
}

func setupHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux setup

Print config keybinds for tmux
`)
}
