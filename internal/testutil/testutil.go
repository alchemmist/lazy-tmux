package testutil

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func SkipIfNotIntegration(t *testing.T) {
	t.Helper()

	v, err := strconv.ParseBool(os.Getenv("ENABLE_INTEGRATION_TESTS"))
	if err != nil || !v {
		t.Skip(
			"ENABLE_INTEGRATION_TESTS is not true. Run integration tests with `make integration-test`",
		)
	}
}

func RequireTMux(t *testing.T) {
	t.Helper()

	if !HasTMux() {
		t.Skip("tmux not installed")
	}
}

func RequireFZF(t *testing.T) {
	t.Helper()

	if !HasFZF() {
		t.Skip("fzf not installed")
	}
}

func HasTMux() bool {
	cmd := exec.CommandContext(context.Background(), "tmux", "-V")
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}

func HasFZF() bool {
	cmd := exec.CommandContext(context.Background(), "fzf", "--version")
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}
