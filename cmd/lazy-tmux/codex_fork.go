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

const codexForkIDLength = 8

func runCodexFork(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdCodexFork, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	pane := flags.String("pane", os.Getenv("TMUX_PANE"), "target tmux pane")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")
	codexBin := flags.String("codex-bin", "codex", "Codex binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			codexForkHelp(stdout)

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
	err = createCodexForkWindow(
		client,
		config.ExpandHome(cfg.Integrations.Codex.Home),
		config.ExpandHome(*codexBin),
		strings.TrimSpace(*pane),
	)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	return 0
}

func createCodexForkWindow(client *tmux.Client, codexHome, codexBin, pane string) error {
	paneSnapshot, err := client.CapturePane(pane)
	if err != nil {
		return fmt.Errorf("capture pane: %w", err)
	}

	sessionID, ok := codex.New(codexHome).SessionID(paneSnapshot)
	if !ok {
		return errCodexSessionNotFound
	}

	windowName := "fork-" + sessionID[:min(codexForkIDLength, len(sessionID))]
	command := strings.Join([]string{
		shellQuote(codexBin),
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

func codexForkHelp(writer io.Writer) {
	_, _ = fmt.Fprint(writer, `Usage: lazy-tmux codex-fork [flags]

Fork the Codex session running in a tmux pane into a named window

Flags:
  -pane         target tmux pane (defaults to $TMUX_PANE or the active pane)
  -tmux-bin     tmux binary
  -codex-bin    Codex binary
`)
}
