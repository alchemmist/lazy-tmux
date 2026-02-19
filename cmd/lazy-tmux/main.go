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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg := config.Default()
	if len(args) < 1 {
		usage(stdout)
		return 2
	}

	var err error
	switch args[0] {
	case "save":
		err = runSave(cfg, args[1:])
	case "restore":
		err = runRestore(cfg, args[1:])
	case "picker":
		err = runPicker(cfg, args[1:])
	case "bootstrap":
		err = runBootstrap(cfg, args[1:])
	case "daemon":
		err = runDaemon(cfg, args[1:])
	case "list":
		err = runList(cfg, args[1:], stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "lazy-tmux: unknown command: %s\n", args[0])
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "lazy-tmux: %s\n", formatError(err))
		return 1
	}
	return 0
}

func runSave(base config.Config, args []string) error {
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "save all sessions")
	session := fs.String("session", "", "save specific session")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

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
	return err
}

func runRestore(base config.Config, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "", "session to restore")
	switchClient := fs.Bool("switch", true, "switch active client to restored session")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*session) == "" {
		return errors.New("restore requires --session")
	}

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)
	return a.Restore(strings.TrimSpace(*session), *switchClient)
}

func runPicker(base config.Config, args []string) error {
	fs := flag.NewFlagSet("picker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)

	session, err := a.SelectWithFZF()
	if err != nil {
		return err
	}
	return a.Restore(session, true)
}

func runBootstrap(base config.Config, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	session := fs.String("session", "last", "session name or 'last'")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	a := app.New(cfg)
	return a.Bootstrap(*session)
}

func runDaemon(base config.Config, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	interval := fs.Duration("interval", base.SaveInterval, "autosave interval")
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	tmuxBin := fs.String("tmux-bin", base.TmuxBin, "tmux binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := base
	cfg.DataDir = *dataDir
	cfg.TmuxBin = *tmuxBin
	cfg.SaveInterval = *interval
	a := app.New(cfg)
	return a.RunDaemon(*interval)
}

func runList(base config.Config, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data-dir", base.DataDir, "snapshot directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := base
	cfg.DataDir = *dataDir
	a := app.New(cfg)
	recs, err := a.ListRecords()
	if err != nil {
		return err
	}
	for _, r := range recs {
		fmt.Fprintf(out, "%s\t%s\t%dw/%dp\n", r.SessionName, r.CapturedAt.Local().Format(time.RFC3339), r.Windows, r.Panes)
	}
	return nil
}

func usage(out io.Writer) {
	fmt.Fprint(out, `lazy-tmux - tmux session snapshots with lazy restore

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

func formatError(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Sprintf("not found: %v", err)
	}
	return err.Error()
}
