package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var exitFunc = os.Exit

var commands = map[string]func(args []string, stdout, stderr io.Writer) int{
	"save":      runSave,
	"restore":   runRestore,
	"picker":    runPicker,
	"bootstrap": runBootstrap,
	"daemon":    runDaemon,
	"list":      runList,
	"setup":     runSetup,
	"wakeup":    runWakeup,
	"sleep":     runSleep,
	"forget":    runForget,
}

var helpFuncs = map[string]func(io.Writer){
	"save":      saveHelp,
	"restore":   restoreHelp,
	"picker":    pickerHelp,
	"bootstrap": bootstrapHelp,
	"daemon":    daemonHelp,
	"list":      listHelp,
	"setup":     setupHelp,
	"wakeup":    func(w io.Writer) { printSessionHelp("wakeup", w) },
	"sleep":     sleepHelp,
	"forget":    func(w io.Writer) { printSessionHelp("forget", w) },
}

func main() {
	exitFunc(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 2
	}

	cmdName := args[0]
	if cmdName == "help" || cmdName == "-h" || cmdName == "--help" {
		if len(args) > 1 {
			help, ok := helpFuncs[args[1]]
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

	cmd, ok := commands[cmdName]
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

Run 'lazy-tmux <command> -h' for more details.
`)
}
