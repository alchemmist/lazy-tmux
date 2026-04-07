package picker

import (
	"os"
)

// isTerminal checks whether f is connected to a real TTY using the idiomatic
// os.ModeCharDevice check. This avoids pulling in golang.org/x/term and matches
// the same check used in internal/tmux/client.go for consistency.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
