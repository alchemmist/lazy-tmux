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
	saveFlags := flag.NewFlagSet("save", flag.ContinueOnError)
	saveFlags.SetOutput(io.Discard)
	all := saveFlags.Bool("all", false, "save all sessions")
	session := saveFlags.String("session", "", "save specific session")
	scrollback := saveFlags.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := saveFlags.Int(
		"scrollback-lines",
		base.Scrollback.Lines,
		"max shell scrollback lines per pane",
	)
	shared := addSharedFlags(saveFlags, base, true)

	if err := saveFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			saveFlags.SetOutput(os.Stdout)
			saveFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse save flags: %w", err)
	}

	if *scrollback && *scrollbackLines <= 0 {
		return fmt.Errorf("save requires --scrollback-lines > 0 when --scrollback is enabled")
	}

	cfg := shared.apply(base)
	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines
	tmuxApp := app.New(cfg)

	var err error

	switch {
	case *all:
		err = tmuxApp.SaveAll()
	case strings.TrimSpace(*session) != "":
		err = tmuxApp.SaveSession(strings.TrimSpace(*session))
	default:
		err = tmuxApp.SaveCurrent()
	}

	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	return nil
}

func runRestore(base config.Config, args []string) error {
	restoreFlags := flag.NewFlagSet("restore", flag.ContinueOnError)
	restoreFlags.SetOutput(io.Discard)
	session := restoreFlags.String("session", "", "session to restore")
	switchClient := restoreFlags.Bool("switch", true, "switch active client to restored session")
	shared := addSharedFlags(restoreFlags, base, true)

	if err := restoreFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			restoreFlags.SetOutput(os.Stdout)
			restoreFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse restore flags: %w", err)
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("restore requires --session")
	}

	tmuxApp := app.New(shared.apply(base))

	if err := tmuxApp.Restore(strings.TrimSpace(*session), *switchClient); err != nil {
		return fmt.Errorf("restore session: %w", err)
	}

	return nil
}

func runPicker(base config.Config, args []string) error {
	pickerFlags := flag.NewFlagSet("picker", flag.ContinueOnError)
	pickerFlags.SetOutput(io.Discard)
	fzfEngine := pickerFlags.Bool("fzf-engine", false, "use fzf engine instead of built-in TUI")
	sessionSort := pickerFlags.String(
		"session-sort",
		"",
		"session sort keys: field[:asc|desc],... (fields: last-used,captured,name,windows,panes)",
	)
	windowSort := pickerFlags.String(
		"window-sort",
		"",
		"window sort keys: field[:asc|desc],... (fields: index,name,panes,cmd)",
	)
	shared := addSharedFlags(pickerFlags, base, true)

	if err := pickerFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			pickerFlags.SetOutput(os.Stdout)
			pickerFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse picker flags: %w", err)
	}

	tmuxApp := app.New(shared.apply(base))

	sortOpts, err := app.ParsePickerSortOptions(*sessionSort, *windowSort)
	if err != nil {
		return fmt.Errorf("parse sort options: %w", err)
	}

	var (
		target app.PickerTarget
		selErr error
	)

	if *fzfEngine {
		session, pickErr := tmuxApp.SelectWithFZFSorted(sortOpts)
		selErr = pickErr
		target = app.PickerTarget{SessionName: session}
	} else {
		target, selErr = tmuxApp.SelectTargetWithTUISorted(sortOpts)
	}

	if selErr != nil {
		return fmt.Errorf("select target: %w", selErr)
	}

	if err := tmuxApp.RestoreTarget(target, true); err != nil {
		return fmt.Errorf("restore target: %w", err)
	}

	return nil
}

func runBootstrap(base config.Config, args []string) error {
	bootstrapFlags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	bootstrapFlags.SetOutput(io.Discard)
	session := bootstrapFlags.String("session", "last", "session name or 'last'")
	shared := addSharedFlags(bootstrapFlags, base, true)

	if err := bootstrapFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			bootstrapFlags.SetOutput(os.Stdout)
			bootstrapFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse bootstrap flags: %w", err)
	}

	a := app.New(shared.apply(base))

	if err := a.Bootstrap(*session); err != nil {
		return fmt.Errorf("bootstrap session: %w", err)
	}

	return nil
}

func runDaemon(base config.Config, args []string) error {
	daemonFlags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	daemonFlags.SetOutput(io.Discard)
	interval := daemonFlags.Duration("interval", base.SaveInterval, "autosave interval")
	scrollback := daemonFlags.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := daemonFlags.Int(
		"scrollback-lines",
		base.Scrollback.Lines,
		"max shell scrollback lines per pane",
	)
	shared := addSharedFlags(daemonFlags, base, true)

	if err := daemonFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			daemonFlags.SetOutput(os.Stdout)
			daemonFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse daemon flags: %w", err)
	}

	if *scrollback && *scrollbackLines <= 0 {
		return fmt.Errorf("daemon requires --scrollback-lines > 0 when --scrollback is enabled")
	}

	cfg := shared.apply(base)
	cfg.SaveInterval = *interval
	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines
	a := app.New(cfg)

	if err := a.RunDaemon(*interval); err != nil {
		return fmt.Errorf("run daemon: %w", err)
	}

	return nil
}

func runList(base config.Config, args []string, stdout io.Writer) error {
	listFlags := flag.NewFlagSet("list", flag.ContinueOnError)
	listFlags.SetOutput(io.Discard)
	shared := addSharedFlags(listFlags, base, false)

	if err := listFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			listFlags.SetOutput(os.Stdout)
			listFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse list flags: %w", err)
	}

	a := app.New(shared.apply(base))

	recs, err := a.ListRecords()
	if err != nil {
		return fmt.Errorf("list records: %w", err)
	}

	for _, record := range recs {
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%dw/%dp\n",
			record.SessionName,
			record.CapturedAt.Local().Format(time.RFC3339),
			record.Windows,
			record.Panes,
		)
	}

	return nil
}

func runWakeup(base config.Config, args []string) error {
	wakeupFlags := flag.NewFlagSet("wakeup", flag.ContinueOnError)
	wakeupFlags.SetOutput(io.Discard)
	session := wakeupFlags.String("session", "", "session to wakeup")
	shared := addSharedFlags(wakeupFlags, base, true)

	if err := wakeupFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			wakeupFlags.SetOutput(os.Stdout)
			wakeupFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse wakeup flags: %w", err)
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("wakeup requires --session")
	}

	a := app.New(shared.apply(base))

	if err := a.Wakeup(strings.TrimSpace(*session)); err != nil {
		return fmt.Errorf("wakeup session: %w", err)
	}

	return nil
}

func runSleep(base config.Config, args []string) error {
	sleepFlags := flag.NewFlagSet("sleep", flag.ContinueOnError)
	sleepFlags.SetOutput(io.Discard)
	session := sleepFlags.String("session", "", "session to sleep")
	shared := addSharedFlags(sleepFlags, base, true)

	if err := sleepFlags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			sleepFlags.SetOutput(os.Stdout)
			sleepFlags.Usage()

			return nil
		}

		return fmt.Errorf("parse sleep flags: %w", err)
	}

	if strings.TrimSpace(*session) == "" {
		return fmt.Errorf("sleep requires --session")
	}

	a := app.New(shared.apply(base))

	if err := a.Sleep(strings.TrimSpace(*session)); err != nil {
		return fmt.Errorf("sleep session: %w", err)
	}

	return nil
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
	fmt.Fprint(
		w,
		`run-shell -b 'lazy-tmux daemon --interval 3m --scrollback>/tmp/lazy-tmux.log 2>&1 `+
			`|| tmux display-message "lazy-tmux daemon already running"'
bind-key f display-popup -w 75% -h 85% -E 'lazy-tmux picker'
bind-key C-s run-shell 'lazy-tmux save --all --scrollback && tmux display-message "All sessions saved successfully!"'
`,
	)
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

func writeFatalErr(writer io.Writer, err error) int {
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(writer, "lazy-tmux: not found: %v\n", err)
		return 1
	}

	fmt.Fprintf(writer, "lazy-tmux: %v\n", err)

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
