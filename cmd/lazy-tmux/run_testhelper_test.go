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

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usageTo(stdout)
		return 2
	}

	cfg := config.Default()
	switch args[0] {
	case "help", "-h", "--help":
		usageTo(stdout)
		return 0
	case "restore":
		fs := flag.NewFlagSet("restore", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		session := fs.String("session", "", "session to restore")
		_ = fs.Parse(args[1:])
		if strings.TrimSpace(*session) == "" {
			fmt.Fprintf(stderr, "lazy-tmux: restore requires --session\n")
			return 1
		}
		return 0
	case "bootstrap":
		fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		session := fs.String("session", "last", "session name or 'last'")
		dataDir := fs.String("data-dir", cfg.DataDir, "snapshot directory")
		tmuxBin := fs.String("tmux-bin", cfg.TmuxBin, "tmux binary")
		_ = fs.Parse(args[1:])

		cfg.DataDir = *dataDir
		cfg.TmuxBin = *tmuxBin
		a := app.New(cfg)
		if err := a.Bootstrap(*session); err != nil {
			return writeFatalErr(stderr, err)
		}
		return 0
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dataDir := fs.String("data-dir", cfg.DataDir, "snapshot directory")
		_ = fs.Parse(args[1:])

		cfg.DataDir = *dataDir
		a := app.New(cfg)
		recs, err := a.ListRecords()
		if err != nil {
			return writeFatalErr(stderr, err)
		}
		for _, r := range recs {
			fmt.Fprintf(stdout, "%s\t%s\t%dw/%dp\n", r.SessionName, r.CapturedAt.Local().Format(time.RFC3339), r.Windows, r.Panes)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "lazy-tmux: unknown command: %s\n", args[0])
		return 1
	}
}

func usageTo(w io.Writer) {
	fmt.Fprint(w, `lazy-tmux - tmux session snapshots with lazy restore

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
  --fzf-engine  Use fzf backend instead of built-in TUI
`)
}

func writeFatalErr(w io.Writer, err error) int {
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(w, "lazy-tmux: not found: %v\n", err)
		return 1
	}
	fmt.Fprintf(w, "lazy-tmux: %v\n", err)
	return 1
}
