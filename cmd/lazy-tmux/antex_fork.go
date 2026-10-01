package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

const antexForkIDLength = 8

func runAntexFork(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdAntexFork, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	pane := flags.String("pane", os.Getenv("TMUX_PANE"), "target tmux pane")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")
	antexBin := flags.String("antex-bin", "antex", "Antex binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			antexForkHelp(stdout)

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
	err = createAntexForkWindow(
		client,
		config.ExpandHome(cfg.Integrations.Antex.Home),
		config.ExpandHome(*antexBin),
		strings.TrimSpace(*pane),
	)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	return 0
}

func createAntexForkWindow(client *tmux.Client, antexHome, antexBin, pane string) error {
	paneSnapshot, err := client.CapturePane(pane)
	if err != nil {
		return fmt.Errorf("capture pane: %w", err)
	}

	sessionID := strings.TrimSpace(paneSnapshot.Meta[snapshot.AntexSessionIDMetaKey])
	if !antex.New(antexHome).Matches(paneSnapshot) || sessionID == "" {
		return errAntexSessionNotFound
	}

	windowName := "fork-" + sessionID[:min(antexForkIDLength, len(sessionID))]
	command := strings.Join([]string{
		shellQuote(antexBin),
		"fork",
		shellQuote(sessionID),
	}, " ")
	_, err = client.Output(
		"new-window",
		"-n",
		windowName,
		"-c",
		paneSnapshot.CurrentPath,
		command,
	)
	if err != nil {
		return fmt.Errorf("create fork window: %w", err)
	}

	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func antexForkHelp(writer io.Writer) {
	_, _ = fmt.Fprint(writer, `Usage: lazy-tmux antex-fork [flags]

Fork the Antex session running in a tmux pane into a named window

Flags:
  -pane         target tmux pane (defaults to $TMUX_PANE or the active pane)
  -tmux-bin     tmux binary
  -antex-bin    Antex binary
`)
}
