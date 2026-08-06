package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/codex"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func runCodexSession(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdCodexSession, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	pane := flags.String("pane", os.Getenv("TMUX_PANE"), "target tmux pane")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			codexSessionHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}
	if flags.NArg() != 0 {
		writeErr(stderr, errUnexpectedArguments)

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if flagPassed(flags, "tmux-bin") {
		cfg.TmuxBin = *tmuxBin
	}

	client := tmux.NewClient(config.ExpandHome(cfg.TmuxBin))
	paneSnapshot, err := client.CapturePane(strings.TrimSpace(*pane))
	if err != nil {
		writeErr(stderr, fmt.Errorf("capture pane: %w", err))

		return 1
	}

	sessionID, ok := codex.New(config.ExpandHome(cfg.Integrations.Codex.Home)).SessionID(
		paneSnapshot,
	)
	if !ok {
		writeErr(stderr, errCodexSessionNotFound)

		return 1
	}

	_, _ = fmt.Fprintln(stdout, sessionID)

	return 0
}

func codexSessionHelp(writer io.Writer) {
	_, _ = fmt.Fprint(writer, `Usage: lazy-tmux codex-session [flags]

Print the Codex session ID running in a tmux pane

Flags:
  -pane         target tmux pane (defaults to $TMUX_PANE or the active pane)
  -tmux-bin     tmux binary
`)
}
