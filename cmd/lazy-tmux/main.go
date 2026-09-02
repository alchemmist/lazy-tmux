package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	errUnknownCommand          = errors.New("unknown command")
	errUnknownConfigSubcommand = errors.New("unknown config subcommand")
	errUnexpectedArguments     = errors.New("unexpected arguments")
	errScrollbackLinesInvalid  = errors.New("scrollback requires scrollback lines > 0")
	errHookUsage               = errors.New("usage: lazy-tmux hook <claude-status|theme> [flags]")
	errUnknownHook             = errors.New("unknown hook")
	errInvalidHookState        = errors.New("invalid --state")
	errWindowsRequiresFzf      = errors.New(
		"--windows requires --fzf-engine (the TUI already lists windows)",
	)
	errSessionsOnlyRequiresTUI = errors.New("sessions-only mode requires the built-in TUI")
	errRequiresSession         = errors.New("requires --session")
	errCodexSessionNotFound    = errors.New("codex session not found for pane")
)

//nolint:gochecknoglobals // test seam: CLI tests stub process exit
var exitFunc = os.Exit

const (
	cmdVersion      = "version"
	cmdSave         = "save"
	cmdRestore      = "restore"
	cmdPicker       = "picker"
	cmdBootstrap    = "bootstrap"
	cmdDaemon       = "daemon"
	cmdList         = "list"
	cmdSetup        = "setup"
	cmdWakeup       = "wakeup"
	cmdSleep        = "sleep"
	cmdForget       = "forget"
	cmdConfig       = "config"
	cmdHook         = "hook"
	cmdClaudeHooks  = "claude-hooks"
	cmdCodexSession = "codex-session"
	cmdCodexFork    = "codex-fork"
)

const flagHelp = "--help"

const exitUsage = 2

func commands() map[string]func(args []string, stdout, stderr io.Writer) int {
	return map[string]func(args []string, stdout, stderr io.Writer) int{
		cmdVersion:      runVersionCmd,
		cmdSave:         runSave,
		cmdRestore:      runRestore,
		cmdPicker:       runPicker,
		cmdBootstrap:    runBootstrap,
		cmdDaemon:       runDaemon,
		cmdList:         runList,
		cmdSetup:        runSetup,
		cmdWakeup:       runWakeup,
		cmdSleep:        runSleep,
		cmdForget:       runForget,
		cmdConfig:       runConfig,
		cmdHook:         runHook,
		cmdClaudeHooks:  runClaudeHooks,
		cmdCodexSession: runCodexSession,
		cmdCodexFork:    runCodexFork,
	}
}

func helpFuncs() map[string]func(io.Writer) {
	return map[string]func(io.Writer){
		cmdVersion:      versionHelp,
		cmdSave:         saveHelp,
		cmdRestore:      restoreHelp,
		cmdPicker:       pickerHelp,
		cmdBootstrap:    bootstrapHelp,
		cmdDaemon:       daemonHelp,
		cmdList:         listHelp,
		cmdSetup:        setupHelp,
		cmdWakeup:       func(w io.Writer) { printSessionHelp(cmdWakeup, true, w) },
		cmdSleep:        sleepHelp,
		cmdForget:       func(w io.Writer) { printSessionHelp(cmdForget, false, w) },
		cmdConfig:       configHelp,
		cmdHook:         hookHelp,
		cmdClaudeHooks:  claudeHooksHelp,
		cmdCodexSession: codexSessionHelp,
		cmdCodexFork:    codexForkHelp,
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
				writeErr(stderr, fmt.Errorf("%w: %s", errUnknownCommand, args[1]))

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
		writeErr(stderr, fmt.Errorf("%w: %s", errUnknownCommand, cmdName))

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

const asciiMoon = `      ▄████▄
    ▄██▀▀        ·
   ▐██▘       ✦
   ▐██▌
   ▐██▖          ·
    ▀██▄▄
      ▀████▀`

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `%s

lazy-tmux - tmux session snapshots with lazy restore

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
  codex-session  Print the Codex session ID running in a tmux pane
  codex-fork     Fork the Codex session running in a tmux pane
  claude-hooks  Install or remove Claude Code status hooks
  hook       Internal hook entrypoints (used by Claude Code)
  version    Print the version

Run 'lazy-tmux <command> -h' for more details.
`, asciiMoon)
}
