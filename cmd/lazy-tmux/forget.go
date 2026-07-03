package main

import (
	"io"

	"github.com/alchemmist/lazy-tmux/internal/app"
)

func runForget(args []string, stdout, stderr io.Writer) int {
	return runSessionOp(args, stdout, stderr, cmdForget, false, func(a *app.App, s string) error {
		return a.Forget(s)
	})
}
