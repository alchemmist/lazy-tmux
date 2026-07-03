package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

// loadConfig builds the effective configuration from defaults plus the TOML
// config file. CLI flags are layered on top by each command afterwards. When
// the config file is malformed it writes the error and returns ok=false so the
// command aborts instead of silently running with defaults.
func loadConfig(stderr io.Writer) (config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		writeErr(stderr, fmt.Errorf("load config: %w", err))

		return cfg, false
	}

	return cfg, true
}

// applyDirBinOverrides layers the --data-dir and --tmux-bin flag values on
// top of cfg when they were explicitly passed on the command line.
func applyDirBinOverrides(flags *flag.FlagSet, cfg *config.Config, dataDir, tmuxBin *string) {
	if flagPassed(flags, "data-dir") {
		cfg.DataDir = *dataDir
	}

	if flagPassed(flags, "tmux-bin") {
		cfg.TmuxBin = *tmuxBin
	}
}

// applyScrollbackOverrides layers the --scrollback and --scrollback-lines flag
// values on top of cfg and validates the result, returning
// errScrollbackLinesInvalid when scrollback is enabled with a non-positive
// line count.
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

// flagPassed reports whether the named flag was explicitly set on the command
// line. It lets file-config values survive when a flag is not provided, while
// still letting an explicit flag override the file.
func flagPassed(flags *flag.FlagSet, name string) bool {
	passed := false

	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			passed = true
		}
	})

	return passed
}
