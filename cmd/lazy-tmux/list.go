package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/app"
)

func runList(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdList, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	dataDir := flags.String("data-dir", "", "snapshot directory")
	tmuxBin := flags.String("tmux-bin", "", "tmux binary")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			listHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}

	if flagPassed(flags, "data-dir") {
		cfg.DataDir = *dataDir
	}

	if flagPassed(flags, "tmux-bin") {
		cfg.TmuxBin = *tmuxBin
	}

	a := app.New(cfg)

	recs, err := a.ListRecords()
	if err != nil {
		writeErr(stderr, fmt.Errorf("list records: %w", err))

		return 1
	}

	for _, rec := range recs {
		_, _ = fmt.Fprintf(
			stdout,
			"%s\t%s\t%dw/%dp\n",
			rec.SessionName,
			rec.CapturedAt.Local().Format(time.RFC3339),
			rec.Windows,
			rec.Panes,
		)
	}

	return 0
}

func listHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: lazy-tmux list [flags]

List saved sessions

Flags:
  -data-dir     snapshot directory
  -tmux-bin     tmux binary
`)
}
