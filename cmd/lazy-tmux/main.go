package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

var (
	exitFunc    = os.Exit
	fatalOutput = io.Writer(os.Stderr)
)

type sharedFlags struct {
	dataDir *string
	tmuxBin *string
}

func main() {
	exitFunc(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usageTo(stdout)
		return 2
	}

	cfg := config.Default()

	switch args[0] {
	case "save":
		if err := runSave(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "restore":
		if err := runRestore(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "picker":
		if err := runPicker(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "bootstrap":
		if err := runBootstrap(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "daemon":
		if err := runDaemon(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "list":
		if err := runList(cfg, args[1:], stdout); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "setup":
		setupConfigTo(stdout)
		return 0
	case "wakeup":
		if err := runWakeup(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "sleep":
		if err := runSleep(cfg, args[1:]); err != nil {
			return writeFatalErr(stderr, err)
		}

		return 0
	case "help", "-h", "--help":
		usageTo(stdout)
		return 0
	default:
		return writeFatalErr(stderr, fmt.Errorf("unknown command: %s", args[0]))
	}
}

func runSave(base config.Config, args []string) error {
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "save all sessions")
	session := fs.String("session", "", "save specific session")
	scrollback := fs.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := fs.Int("scrollback-lines", base.Scrollback.Lines, "max shell scrollback lines per pane")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	if *scrollback && *scrollbackLines <= 0 {
		return fmt.Errorf("save requires --scrollback-lines > 0 when --scrollback is enabled")
	}

	cfg := shared.apply(base)
	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines
	a := app.New(cfg)

	var err error

	switch {
	case *all:
		err = a.SaveAll()
	case strings.TrimSpace(*session) != "":
		err = a.SaveSession(strings.TrimSpace(*session))
	default:
		err = a.SaveCurrent()
	}

	return err
}

func runRestore(base config.Config, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "session to restore")
	switchClient := fs.Bool("switch", true, "switch active client to restored session")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("restore requires --session")
	}

	a := app.New(shared.apply(base))

	return a.Restore(strings.TrimSpace(*session), *switchClient)
}

func runPicker(base config.Config, args []string) error {
	fs := flag.NewFlagSet("picker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fzfEngine := fs.Bool("fzf-engine", false, "use fzf engine instead of built-in TUI")
	sessionSort := fs.String("session-sort", "", "session sort keys: field[:asc|desc],... (fields: last-used,captured,name,windows,panes)")
	windowSort := fs.String("window-sort", "", "window sort keys: field[:asc|desc],... (fields: index,name,panes,cmd)")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	a := app.New(shared.apply(base))

	sortOpts, err := app.ParsePickerSortOptions(*sessionSort, *windowSort)
	if err != nil {
		return err
	}

	var (
		target app.PickerTarget
		selErr error
	)

	if *fzfEngine {
		session, pickErr := a.SelectWithFZFSorted(sortOpts)
		selErr = pickErr
		target = app.PickerTarget{SessionName: session}
	} else {
		target, selErr = a.SelectTargetWithTUISorted(sortOpts)
	}

	if selErr != nil {
		return selErr
	}

	return a.RestoreTarget(target, true)
}

func runBootstrap(base config.Config, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "last", "session name or 'last'")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	a := app.New(shared.apply(base))

	return a.Bootstrap(*session)
}

func runDaemon(base config.Config, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	interval := fs.Duration("interval", base.SaveInterval, "autosave interval")
	scrollback := fs.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := fs.Int("scrollback-lines", base.Scrollback.Lines, "max shell scrollback lines per pane")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	if *scrollback && *scrollbackLines <= 0 {
		return fmt.Errorf("daemon requires --scrollback-lines > 0 when --scrollback is enabled")
	}

	cfg := shared.apply(base)
	cfg.SaveInterval = *interval
	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines
	a := app.New(cfg)

	return a.RunDaemon(*interval)
}

func runList(base config.Config, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	shared := addSharedFlags(fs, base, false)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	a := app.New(shared.apply(base))

	recs, err := a.ListRecords()
	if err != nil {
		return err
	}

	for _, r := range recs {
		fmt.Fprintf(stdout, "%s\t%s\t%dw/%dp\n", r.SessionName, r.CapturedAt.Local().Format(time.RFC3339), r.Windows, r.Panes)
	}

	return nil
}

func runWakeup(base config.Config, args []string) error {
	fs := flag.NewFlagSet("wakeup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "session to wakeup")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("wakeup requires --session")
	}

	a := app.New(shared.apply(base))

	return a.Wakeup(strings.TrimSpace(*session))
}

func runSleep(base config.Config, args []string) error {
	fs := flag.NewFlagSet("sleep", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "session to sleep")
	shared := addSharedFlags(fs, base, true)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()

			return nil
		}

		return err
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("sleep requires --session")
	}

	a := app.New(shared.apply(base))

	return a.Sleep(strings.TrimSpace(*session))
}

func usage() {
	usageTo(os.Stdout)
}

func usageTo(w io.Writer) {
	fmt.Fprint(w, `lazy-tmux - tmux session snapshots with lazy restore

Usage:
  lazy-tmux <command> [flags]

Commands:
  save       Save current or selected sessions
  restore    Restore one session from disk
  wakeup     Restore a saved session (lazy load) without switching clients
  sleep      Save and close a running session
  picker     Open session picker and restore selected session (default: TUI)
  bootstrap  Restore one session at tmux startup (default: last)
  daemon     Periodically save all sessions
  list       List saved sessions
  setup      Print config keybinds for tmux

Picker flags:
  --fzf-engine             Use fzf backend instead of built-in TUI
  --session-sort EXPR      Session sort (field[:asc|desc],...) fields: last-used,captured,name,windows,panes
  --window-sort EXPR       Window sort (field[:asc|desc],...) fields: index,name,panes,cmd

Save/daemon flags:
  --scrollback             Capture shell pane scrollback (opt-in)
  --scrollback-lines N     Max captured lines per shell pane (default: 5000)
`)
}

func setupConfig() {
	setupConfigTo(os.Stdout)
}

func setupConfigTo(w io.Writer) {
	fmt.Fprint(w, `run-shell -b 'lazy-tmux daemon --interval 3m --scrollback>/tmp/lazy-tmux.log 2>&1 || tmux display-message "lazy-tmux daemon already running"'
bind-key f display-popup -w 75% -h 85% -E 'lazy-tmux picker'
bind-key C-s run-shell 'lazy-tmux save --all --scrollback && tmux display-message "All sessions saved successfully!"'
`)
}

func fatalErr(err error) {
	if errors.Is(err, os.ErrNotExist) {
		fatalf("not found: %v", err)
	}

	fatalf("%v", err)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(fatalOutput, "lazy-tmux: "+format+"\n", args...)
	exitFunc(1)
}

func writeFatalErr(w io.Writer, err error) int {
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(w, "lazy-tmux: not found: %v\n", err)
		return 1
	}

	fmt.Fprintf(w, "lazy-tmux: %v\n", err)

	return 1
}

func addSharedFlags(fs *flag.FlagSet, base config.Config, withTmux bool) sharedFlags {
	flags := sharedFlags{
		dataDir: fs.String("data-dir", base.DataDir, "snapshot directory"),
	}
	if withTmux {
		flags.tmuxBin = fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	}

	return flags
}

func (f sharedFlags) apply(base config.Config) config.Config {
	cfg := base
	if f.dataDir != nil {
		cfg.DataDir = *f.dataDir
	}

	if f.tmuxBin != nil {
		cfg.TmuxBin = *f.tmuxBin
	}

	return cfg
}
