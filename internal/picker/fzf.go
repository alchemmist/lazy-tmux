package picker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var ErrNoSessions = errors.New("no sessions available")

func ChooseSessionFZF(records []snapshot.Record) (string, error) {
	if len(records) == 0 {
		return "", ErrNoSessions
	}

	var input bytes.Buffer

	for _, r := range records {
		line := fmt.Sprintf(
			"%s\t%s\t%dw\n",
			r.SessionName,
			r.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			r.Windows,
		)
		input.WriteString(line)
	}

	args := []string{
		"fzf",
		"--delimiter", "\t",
		"--with-nth", "1,2,3",
	}

	if isTerminal(os.Stdout) {
		args = append(args,
			"--prompt", "lazy-tmux> ",
			"--height", "100%",
			"--layout", "reverse",
		)
	} else {
		args = append(args, "--filter", "")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = &input

	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("fzf selection timed out")
		}

		return "", fmt.Errorf("fzf selection canceled or failed: %w", err)
	}

	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", fmt.Errorf("no session selected")
	}

	parts := strings.Split(selected, "\t")
	if strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("invalid fzf output")
	}

	return parts[0], nil
}
