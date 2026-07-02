package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		configHelp(stderr)

		return 1
	}

	switch args[0] {
	case "gen":
		return runConfigGen(args[1:], stdout, stderr)
	case "show":
		return runConfigShow(args[1:], stdout, stderr)
	case "-h", flagHelp, "help":
		configHelp(stdout)

		return 0
	default:
		writeErr(stderr, fmt.Errorf("%w: %s", errUnknownConfigSubcommand, args[0]))
		configHelp(stderr)

		return 1
	}
}

func runConfigGen(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config gen", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	path := flags.String("path", "", "config file path (default: the path lazy-tmux reads)")
	force := flags.Bool("force", false, "overwrite an existing file")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			configHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if flags.NArg() != 0 {
		writeErr(stderr, fmt.Errorf("%w: %v", errUnexpectedArguments, flags.Args()))

		return 1
	}

	written, err := config.GenerateConfig(*path, *force)
	if err != nil {
		writeErr(stderr, fmt.Errorf("config gen: %w", err))

		return 1
	}

	_, _ = fmt.Fprintf(stdout, "wrote config to %s\n", written)

	return 0
}

func runConfigShow(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			configHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	if flags.NArg() != 0 {
		writeErr(stderr, fmt.Errorf("%w: %v", errUnexpectedArguments, flags.Args()))

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}

	// Report which file (if any) the effective config came from.
	source := "built-in defaults (no config file path)"

	path := config.DefaultConfigPath()
	if path != "" {
		_, statErr := os.Stat(path)
		if statErr == nil {
			source = path
		} else {
			source = path + " (not found — using built-in defaults)"
		}
	}

	_, _ = fmt.Fprintf(stdout, "# config source: %s\n\n", source)
	_, _ = fmt.Fprint(stdout, cfg.Render())

	return 0
}

func configHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux config <subcommand>

Subcommands:
  gen [--path FILE] [--force]   Write a base config file (default: the path lazy-tmux reads)
  show                          Print the effective config lazy-tmux actually reads
`)
}
