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
