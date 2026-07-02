package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

//nolint:gochecknoglobals // test seam: CLI tests stub process exit
var exitFunc = os.Exit

// Subcommand names, shared by the dispatch tables, flag sets and help text.
const (
	cmdVersion     = "version"
	cmdSave        = "save"
	cmdRestore     = "restore"
	cmdPicker      = "picker"
	cmdBootstrap   = "bootstrap"
	cmdDaemon      = "daemon"
	cmdList        = "list"
	cmdSetup       = "setup"
	cmdWakeup      = "wakeup"
	cmdSleep       = "sleep"
	cmdForget      = "forget"
	cmdConfig      = "config"
	cmdHook        = "hook"
	cmdClaudeHooks = "claude-hooks"
)

// flagHelp is the long help flag every subcommand honors.
const flagHelp = "--help"

// exitUsage is the exit code for malformed invocations (missing or unknown
// command, bad hook usage) — distinct from 1, which is a failed operation.
const exitUsage = 2

// commands maps each subcommand name to its runner.
func commands() map[string]func(args []string, stdout, stderr io.Writer) int {
	return map[string]func(args []string, stdout, stderr io.Writer) int{
		cmdVersion:     runVersionCmd,
		cmdSave:        runSave,
		cmdRestore:     runRestore,
		cmdPicker:      runPicker,
		cmdBootstrap:   runBootstrap,
		cmdDaemon:      runDaemon,
		cmdList:        runList,
		cmdSetup:       runSetup,
		cmdWakeup:      runWakeup,
		cmdSleep:       runSleep,
		cmdForget:      runForget,
		cmdConfig:      runConfig,
		cmdHook:        runHook,
		cmdClaudeHooks: runClaudeHooks,
	}
}

// helpFuncs maps each subcommand name to its help printer.
func helpFuncs() map[string]func(io.Writer) {
	return map[string]func(io.Writer){
		cmdVersion:     versionHelp,
		cmdSave:        saveHelp,
		cmdRestore:     restoreHelp,
		cmdPicker:      pickerHelp,
		cmdBootstrap:   bootstrapHelp,
		cmdDaemon:      daemonHelp,
		cmdList:        listHelp,
		cmdSetup:       setupHelp,
		cmdWakeup:      func(w io.Writer) { printSessionHelp(cmdWakeup, true, w) },
		cmdSleep:       sleepHelp,
		cmdForget:      func(w io.Writer) { printSessionHelp(cmdForget, false, w) },
		cmdConfig:      configHelp,
		cmdHook:        hookHelp,
		cmdClaudeHooks: claudeHooksHelp,
	}
}

func main() {
	exitFunc(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)

		return exitUsage
	}

	cmdName := args[0]
	if cmdName == "-v" || cmdName == "--version" {
		return runVersion(stdout)
	}

	if cmdName == "help" || cmdName == "-h" || cmdName == flagHelp {
		if len(args) > 1 {
			help, ok := helpFuncs()[args[1]]
			if !ok {
				writeErr(stderr, fmt.Errorf("unknown command: %s", args[1]))

				return 1
			}

			help(stdout)
		} else {
			printUsage(stdout)
		}

		return 0
	}

	cmd, ok := commands()[cmdName]
	if !ok {
		writeErr(stderr, fmt.Errorf("unknown command: %s", cmdName))

		return 1
	}

	return cmd(args[1:], stdout, stderr)
}

func writeErr(w io.Writer, err error) {
	if errors.Is(err, os.ErrNotExist) {
		_, _ = fmt.Fprintf(w, "lazy-tmux: not found: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(w, "lazy-tmux: %v\n", err)
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `lazy-tmux - tmux session snapshots with lazy restore

Usage:
  lazy-tmux <command> [flags]

Commands:
  save       Save current or selected sessions
  restore    Restore one session from disk
  wakeup     Restore a saved session without switching clients
  sleep      Save and close a running session
  forget     Remove a stored session from disk
  picker     Open session picker and restore selected session
  bootstrap  Restore one session at tmux startup
  daemon     Periodically save all sessions
  list       List saved sessions
  setup      Print config keybinds for tmux
  config     Generate (gen) or show the config file
  claude-hooks  Install or remove Claude Code status hooks
  hook       Internal hook entrypoints (used by Claude Code)
  version    Print the version

Run 'lazy-tmux <command> -h' for more details.
`)
}
