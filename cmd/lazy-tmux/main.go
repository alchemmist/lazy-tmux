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
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	_ = fs.Parse(args)

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
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
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	_ = fs.Parse(args)
	if strings.TrimSpace(*session) == "" {
		fatalf("restore requires --session")
	}

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)
	if err := a.Restore(strings.TrimSpace(*session), *switchClient); err != nil {
		fatalErr(err)
	}
}

func runPicker(base config.Config, args []string) {
	fs := flag.NewFlagSet("picker", flag.ExitOnError)
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	_ = fs.Parse(args)

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)

	session, err := a.SelectWithFZF()
	if err != nil {
		fatalErr(err)
	}
	if err := a.Restore(session, true); err != nil {
		fatalErr(err)
	}
}

func runBootstrap(base config.Config, args []string) {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	session := fs.String("session", "last", "session name or 'last'")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	_ = fs.Parse(args)

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)
	if err := a.Bootstrap(*session); err != nil {
		fatalErr(err)
	}
}

func runDaemon(base config.Config, args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	interval := fs.Duration("interval", base.SaveInterval, "autosave interval")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	_ = fs.Parse(args)

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	cfg.SaveInterval = *interval
	a := app.New(cfg)
	if err := a.RunDaemon(*interval); err != nil {
		fatalErr(err)
	}
}

func runList(base config.Config, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	_ = fs.Parse(args)

	cfg := base
	cfg.DataDir = *dataDir
	a := app.New(cfg)
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
  picker     Open fzf-based selection flow and restore selected session
  bootstrap  Restore one session at tmux startup (default: last)
  daemon     Periodically save all sessions
  list       List saved sessions
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
