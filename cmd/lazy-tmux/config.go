package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

func loadConfig(stderr io.Writer) (config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(stderr, fmt.Errorf("load config: %w", err))

		return cfg, false
	}

	return cfg, true
}

func applyDirBinOverrides(flags *flag.FlagSet, cfg *config.Config, dataDir, tmuxBin *string) {
	if flagPassed(flags, "data-dir") {
		cfg.DataDir = *dataDir
	}

	if flagPassed(flags, "tmux-bin") {
		cfg.TmuxBin = *tmuxBin
	}
}

func applyScrollbackOverrides(
	flags *flag.FlagSet,
	cfg *config.Config,
	enabled *bool,
	lines *int,
) error {
	if flagPassed(flags, "scrollback") {
		cfg.Scrollback.Enabled = *enabled
	}

	if flagPassed(flags, "scrollback-lines") {
		cfg.Scrollback.Lines = *lines
	}

	if cfg.Scrollback.Enabled && cfg.Scrollback.Lines <= 0 {
		return errScrollbackLinesInvalid
	}

	return nil
}

func flagPassed(flags *flag.FlagSet, name string) bool {
	passed := false

	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			passed = true
		}
	})

	return passed
}
