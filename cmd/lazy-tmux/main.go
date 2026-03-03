package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/app"
	"github.com/alchemmist/lazy-tmux/internal/config"
)

type sharedFlags struct {
	dataDir *string
	tmuxBin *string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg := config.Default()
	switch os.Args[1] {
	case "save":
		runSave(cfg, os.Args[2:])
	case "restore":
		runRestore(cfg, os.Args[2:])
	case "picker":
		runPicker(cfg, os.Args[2:])
	case "bootstrap":
		runBootstrap(cfg, os.Args[2:])
	case "daemon":
		runDaemon(cfg, os.Args[2:])
	case "list":
		runList(cfg, os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fatalf("unknown command: %s", os.Args[1])
	}
}

func runSave(base config.Config, args []string) {
	fs := flag.NewFlagSet("save", flag.ExitOnError)
	all := fs.Bool("all", false, "save all sessions")
	session := fs.String("session", "", "save specific session")
	scrollback := fs.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := fs.Int("scrollback-lines", base.Scrollback.Lines, "max shell scrollback lines per pane")
	shared := addSharedFlags(fs, base, true)
	_ = fs.Parse(args)

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
	if err != nil {
		fatalErr(err)
	}
}

func runRestore(base config.Config, args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	session := fs.String("session", "", "session to restore")
	switchClient := fs.Bool("switch", true, "switch active client to restored session")
	shared := addSharedFlags(fs, base, true)
	_ = fs.Parse(args)
	if strings.TrimSpace(*session) == "" {
		fatalf("restore requires --session")
	}

	a := app.New(shared.apply(base))
	if err := a.Restore(strings.TrimSpace(*session), *switchClient); err != nil {
		fatalErr(err)
	}
}

func runPicker(base config.Config, args []string) {
	fs := flag.NewFlagSet("picker", flag.ExitOnError)
	fzfEngine := fs.Bool("fzf-engine", false, "use fzf engine instead of built-in TUI")
	sessionSort := fs.String("session-sort", "", "session sort keys: field[:asc|desc],... (fields: last-used,captured,name,windows,panes)")
	windowSort := fs.String("window-sort", "", "window sort keys: field[:asc|desc],... (fields: index,name,panes,cmd)")
	shared := addSharedFlags(fs, base, true)
	_ = fs.Parse(args)

	a := app.New(shared.apply(base))
	sortOpts, err := app.ParsePickerSortOptions(*sessionSort, *windowSort)
	if err != nil {
		fatalErr(err)
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
		fatalErr(selErr)
	}
	if err := a.RestoreTarget(target, true); err != nil {
		fatalErr(err)
	}
}

func runBootstrap(base config.Config, args []string) {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	session := fs.String("session", "last", "session name or 'last'")
	shared := addSharedFlags(fs, base, true)
	_ = fs.Parse(args)

	a := app.New(shared.apply(base))
	if err := a.Bootstrap(*session); err != nil {
		fatalErr(err)
	}
}

func runDaemon(base config.Config, args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	interval := fs.Duration("interval", base.SaveInterval, "autosave interval")
	scrollback := fs.Bool("scrollback", base.Scrollback.Enabled, "capture shell pane scrollback")
	scrollbackLines := fs.Int("scrollback-lines", base.Scrollback.Lines, "max shell scrollback lines per pane")
	shared := addSharedFlags(fs, base, true)
	_ = fs.Parse(args)

	cfg := shared.apply(base)
	cfg.SaveInterval = *interval
	cfg.Scrollback.Enabled = *scrollback
	cfg.Scrollback.Lines = *scrollbackLines
	a := app.New(cfg)
	if err := a.RunDaemon(*interval); err != nil {
		fatalErr(err)
	}
}

func runList(base config.Config, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	shared := addSharedFlags(fs, base, false)
	_ = fs.Parse(args)

	a := app.New(shared.apply(base))
	recs, err := a.ListRecords()
	if err != nil {
		fatalErr(err)
	}
	for _, r := range recs {
		fmt.Printf("%s\t%s\t%dw/%dp\n", r.SessionName, r.CapturedAt.Local().Format(time.RFC3339), r.Windows, r.Panes)
	}
}

func usage() {
	fmt.Print(`lazy-tmux - tmux session snapshots with lazy restore

Usage:
  lazy-tmux <command> [flags]

Commands:
  save       Save current or selected sessions
  restore    Restore one session from disk
  picker     Open session picker and restore selected session (default: TUI)
  bootstrap  Restore one session at tmux startup (default: last)
  daemon     Periodically save all sessions
  list       List saved sessions

Picker flags:
  --fzf-engine             Use fzf backend instead of built-in TUI
  --session-sort EXPR      Session sort (field[:asc|desc],...) fields: last-used,captured,name,windows,panes
  --window-sort EXPR       Window sort (field[:asc|desc],...) fields: index,name,panes,cmd

Save/daemon flags:
  --scrollback             Capture shell pane scrollback (opt-in)
  --scrollback-lines N     Max captured lines per shell pane (default: 5000)
`)
}

func fatalErr(err error) {
	if errors.Is(err, os.ErrNotExist) {
		fatalf("not found: %v", err)
	}
	fatalf("%v", err)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lazy-tmux: "+format+"\n", args...)
	os.Exit(1)
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
