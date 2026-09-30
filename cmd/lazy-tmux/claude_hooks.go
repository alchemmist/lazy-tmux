package main

import (
	"fmt"
	"io"
)

func runClaudeHooks(args []string, stdout, stderr io.Writer) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			claudeHooksHelp(stdout)

			return 0
		}
	}

	return runIntegrations(append([]string{"setup", "claude"}, args...), stdout, stderr)
}

func claudeHooksHelp(w io.Writer) {
	_, _ = fmt.Fprintln(
		w,
		"Usage: lazy-tmux claude-hooks [--uninstall]\n\n"+
			"Install or remove owned Claude Code lifecycle hooks; alias for integrations setup claude.",
	)
}
