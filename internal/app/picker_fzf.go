package app

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func chooseSessionFZF(records []snapshot.Record) (string, error) {
	var input bytes.Buffer
	for _, r := range records {
		line := fmt.Sprintf("%s\t%s\t%dw\n", r.SessionName, r.CapturedAt.Local().Format("2006-01-02 15:04:05"), r.Windows)
		input.WriteString(line)
	}

	cmd := exec.Command("fzf", "--prompt", "lazy-tmux> ", "--delimiter", "\t", "--with-nth", "1,2,3", "--height", "100%", "--layout", "reverse")
	cmd.Stdin = &input
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("fzf selection canceled or failed: %w", err)
	}

	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", fmt.Errorf("no session selected")
	}
	parts := strings.Split(selected, "\t")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid fzf output")
	}
	return parts[0], nil
}
