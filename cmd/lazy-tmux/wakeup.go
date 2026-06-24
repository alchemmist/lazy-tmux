package main

import (
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
)

func runWakeup(args []string, stdout, stderr io.Writer) int {
	return runSessionOp(args, stdout, stderr, "wakeup", true, func(a *app.App, s string) error {
		return a.Wakeup(s)
	})
}
